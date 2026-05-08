package subscription

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/kymjs/noteapi/internal/config"
	"github.com/kymjs/noteapi/internal/store"
)

const lifetimeYear = 2099

// ExtendAfterPayment 与客户端「未过期则顺延、已过期则从今日起算」一致；按自然月递增。
func ExtendAfterPayment(sub *store.SubscriptionRow, plan string, nowUTC time.Time) (newExpiry time.Time, lifetime bool) {
	months := config.ParsePlanMonths(plan)
	if months <= 0 {
		return time.Time{}, false
	}
	today := dateUTC(nowUTC)
	if sub != nil && sub.IsLifetime {
		return time.Date(lifetimeYear, 12, 31, 0, 0, 0, 0, time.UTC), true
	}
	var anchor time.Time
	if sub != nil && sub.ExpiresAt.Valid {
		expDay := dateUTC(sub.ExpiresAt.Time)
		if !expDay.Before(today) {
			anchor = expDay
		} else {
			anchor = today
		}
	} else {
		anchor = today
	}
	newExpiry = anchor.AddDate(0, months, 0)
	return newExpiry, false
}

func dateUTC(t time.Time) time.Time {
	y, m, d := t.In(time.UTC).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// RowToAPIState 将数据库订阅转为接口返回状态。
func RowToAPIState(sub *store.SubscriptionRow, nowUTC time.Time) (state string, expiresYmd string, isLifetime bool) {
	today := dateUTC(nowUTC)
	if sub == nil {
		return "none", "", false
	}
	if sub.IsLifetime {
		return "lifetime", "", true
	}
	if !sub.ExpiresAt.Valid {
		return "none", "", false
	}
	exp := dateUTC(sub.ExpiresAt.Time)
	if exp.Year() >= lifetimeYear {
		return "lifetime", "", true
	}
	y, m, d := exp.Date()
	expiresYmd = fmt.Sprintf("%04d-%02d-%02d", y, int(m), d)
	if today.After(exp) {
		return "expired", expiresYmd, false
	}
	return "active", expiresYmd, false
}

func NullableDate(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: true}
}
