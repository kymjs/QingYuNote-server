package referral

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/kymjs/noteapi/internal/store"
	"github.com/kymjs/noteapi/internal/subscription"
)

// OnNewUserRegistered 新用户注册：赠送欢迎会员、处理邀请关系。
func OnNewUserRegistered(ctx context.Context, st *store.Store, userID int64, claimToken, inviteePhone string) {
	now := time.Now().UTC()
	if err := grantMembershipDays(ctx, st, userID, store.WelcomeBonusDays, now); err != nil {
		log.Printf("referral welcome bonus user=%d: %v", userID, err)
		return
	}
	if err := st.InsertMembershipGrantRecord(ctx, &store.MembershipGrantRecordParams{
		UserID: userID, Source: store.GrantSourceWelcome, GrantDays: store.WelcomeBonusDays,
	}); err != nil {
		log.Printf("referral welcome grant audit user=%d: %v", userID, err)
	}
	if err := st.GrantWelcomeBonusPending(ctx, userID, now); err != nil {
		log.Printf("referral welcome pending user=%d: %v", userID, err)
	}
	tryBindReferralForUser(ctx, st, userID, claimToken, inviteePhone, now)
}

// OnPhoneBound 用户首次绑定手机号：尝试按手机号匹配官网提交的待处理邀请（三方登录后绑手机场景）。
func OnPhoneBound(ctx context.Context, st *store.Store, userID int64, phone string) {
	now := time.Now().UTC()
	if _, err := st.GetInviterUserIDForInvitee(ctx, userID); err == nil {
		return
	} else if !errors.Is(err, sql.ErrNoRows) {
		log.Printf("referral phone bound lookup user=%d: %v", userID, err)
		return
	}
	tryBindReferralForUser(ctx, st, userID, "", phone, now)
}

func tryBindReferralForUser(ctx context.Context, st *store.Store, inviteeUserID int64, claimToken, inviteePhone string, now time.Time) {
	token := strings.TrimSpace(claimToken)
	phone := store.NormalizeLoginPhoneDigits(inviteePhone)
	if token != "" {
		if err := tryBindReferralClaim(ctx, st, inviteeUserID, token, inviteePhone, now); err == nil {
			return
		} else if !errors.Is(err, sql.ErrNoRows) {
			log.Printf("referral claim user=%d token=%s: %v", inviteeUserID, token, err)
		}
	}
	if phone == "" {
		return
	}
	if err := tryBindReferralClaimByPhone(ctx, st, inviteeUserID, phone, now); err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Printf("referral claim user=%d phone=%s: %v", inviteeUserID, phone, err)
	}
}

func tryBindReferralClaim(ctx context.Context, st *store.Store, inviteeUserID int64, token, inviteePhone string, now time.Time) error {
	claim, err := st.GetReferralClaimByTokenForInvitee(ctx, token)
	if err != nil {
		return err
	}
	return bindReferralClaim(ctx, st, inviteeUserID, claim, inviteePhone, now)
}

func tryBindReferralClaimByPhone(ctx context.Context, st *store.Store, inviteeUserID int64, phone string, now time.Time) error {
	claim, err := st.GetPendingReferralClaimByInviteePhone(ctx, phone, now)
	if err != nil {
		return err
	}
	return bindReferralClaim(ctx, st, inviteeUserID, claim, phone, now)
}

func bindReferralClaim(ctx context.Context, st *store.Store, inviteeUserID int64, claim *store.ReferralClaimRow, inviteePhone string, now time.Time) error {
	if claim.Status == store.ReferralClaimStatusSuccess || claim.UsedAt.Valid {
		return errors.New("claim_already_used")
	}
	if claim.Status == store.ReferralClaimStatusFailed {
		return errors.New("claim_failed")
	}
	if now.Sub(claim.ClaimedAt) > store.ReferralClaimValidWindow {
		_ = st.MarkReferralClaimFailed(ctx, claim.ID)
		return errors.New("claim_expired")
	}
	if claim.InviterUserID == inviteeUserID {
		return errors.New("self_referral")
	}
	claimPhone := ""
	if claim.InviteePhone.Valid {
		claimPhone = store.NormalizeLoginPhoneDigits(claim.InviteePhone.String)
	}
	regPhone := store.NormalizeLoginPhoneDigits(inviteePhone)
	if claimPhone == "" || regPhone == "" || claimPhone != regPhone {
		return errors.New("invitee_phone_mismatch")
	}

	if _, err := st.GetInviterUserIDForInvitee(ctx, inviteeUserID); err == nil {
		return errors.New("invitee_already_referred")
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	if err := st.InsertUserReferral(ctx, claim.InviterUserID, inviteeUserID, now); err != nil {
		return err
	}
	if err := st.MarkReferralClaimSuccess(ctx, claim.ID, inviteeUserID, now); err != nil {
		return err
	}
	if err := grantMembershipDays(ctx, st, claim.InviterUserID, store.ReferralInviterDays, now); err != nil {
		return err
	}
	if err := st.InsertMembershipGrantRecord(ctx, &store.MembershipGrantRecordParams{
		UserID: claim.InviterUserID, Source: store.GrantSourceInviteFriend,
		GrantDays: store.ReferralInviterDays, RelatedUserID: inviteeUserID,
	}); err != nil {
		log.Printf("referral inviter grant audit inviter=%d: %v", claim.InviterUserID, err)
	}
	if err := grantMembershipMonths(ctx, st, inviteeUserID, "monthly", now); err != nil {
		return err
	}
	if err := st.InsertMembershipGrantRecord(ctx, &store.MembershipGrantRecordParams{
		UserID: inviteeUserID, Source: store.GrantSourceInvited,
		GrantMonths: store.ReferralInviteeMonths, RelatedUserID: claim.InviterUserID,
	}); err != nil {
		log.Printf("referral invitee grant audit invitee=%d: %v", inviteeUserID, err)
	}
	return nil
}

func grantMembershipDays(ctx context.Context, st *store.Store, userID int64, days int, now time.Time) error {
	sub, err := st.GetSubscription(ctx, userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		sub = nil
	}
	newExp, lifetime := subscription.ExtendByDays(sub, days, now)
	if lifetime {
		return st.UpsertSubscriptionExpiry(ctx, userID, time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC), true)
	}
	return st.UpsertSubscriptionExpiry(ctx, userID, newExp, false)
}

func grantMembershipMonths(ctx context.Context, st *store.Store, userID int64, plan string, now time.Time) error {
	sub, err := st.GetSubscription(ctx, userID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if errors.Is(err, sql.ErrNoRows) {
		sub = nil
	}
	newExp, lifetime := subscription.ExtendAfterPayment(sub, plan, now)
	if lifetime {
		return st.UpsertSubscriptionExpiry(ctx, userID, time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC), true)
	}
	return st.UpsertSubscriptionExpiry(ctx, userID, newExp, false)
}

// OnInviteeRechargePaid 被邀请人充值成功后，为邀请人发放返利会员。
func OnInviteeRechargePaid(ctx context.Context, st *store.Store, inviteeUserID, orderID int64, planID string) {
	rewardDays := subscription.ReferralInviterRewardDays(planID)
	if rewardDays <= 0 {
		return
	}
	exists, err := st.ReferralRechargeRewardExists(ctx, orderID)
	if err != nil || exists {
		return
	}
	inviterID, err := st.GetInviterUserIDForInvitee(ctx, inviteeUserID)
	if errors.Is(err, sql.ErrNoRows) {
		return
	}
	if err != nil {
		log.Printf("referral inviter lookup invitee=%d: %v", inviteeUserID, err)
		return
	}
	now := time.Now().UTC()
	if err := grantMembershipDays(ctx, st, inviterID, rewardDays, now); err != nil {
		log.Printf("referral recharge reward inviter=%d order=%d: %v", inviterID, orderID, err)
		return
	}
	if err := st.InsertMembershipGrantRecord(ctx, &store.MembershipGrantRecordParams{
		UserID: inviterID, Source: store.GrantSourceInviteRecharge,
		GrantDays: rewardDays, RelatedUserID: inviteeUserID, OrderID: orderID,
	}); err != nil {
		log.Printf("referral recharge grant audit inviter=%d order=%d: %v", inviterID, orderID, err)
	}
	if err := st.InsertReferralRechargeReward(ctx, inviterID, inviteeUserID, orderID, planID, rewardDays, now); err != nil {
		log.Printf("referral recharge audit inviter=%d order=%d: %v", inviterID, orderID, err)
	}
}

// NewClaimToken 生成 32 字符十六进制邀请领取令牌。
func NewClaimToken() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
