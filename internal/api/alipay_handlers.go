package api

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/kymjs/noteapi/internal/alipayapp"
	"github.com/kymjs/noteapi/internal/config"
	"github.com/kymjs/noteapi/internal/store"
	alipay "github.com/smartwalle/alipay/v3"
)

const alipayNotifyRelPath = "/api/v1/webhooks/alipay/notify"

func (s *Server) alipayNotifyAbsURL() string {
	return strings.TrimRight(s.Cfg.PublicBaseURL, "/") + alipayNotifyRelPath
}

func alipaySubjectForPlan(planID string) string {
	switch strings.TrimSpace(planID) {
	case "monthly":
		return "轻羽云服务会员（月付）"
	case "half_year":
		return "轻羽云服务会员（半年付）"
	case "yearly":
		return "轻羽云服务会员（年付）"
	default:
		return "轻羽云服务会员"
	}
}

func amountFenToAlipayYuan(fen int) string {
	return fmt.Sprintf("%.2f", float64(fen)/100.0)
}

func parseAlipayTotalFen(totalAmount string) (int, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(totalAmount), 64)
	if err != nil || f < 0 {
		return 0, false
	}
	return int(math.Round(f * 100)), true
}

// handleAlipayAppPaySign 为待支付订单生成 alipay.trade.app.pay 的 orderStr（证书加签）。
func (s *Server) handleAlipayAppPaySign(w http.ResponseWriter, r *http.Request, uid int64) {
	if !s.Cfg.AlipayAppPayConfigured() {
		s.Cfg.LogAlipayDiagnostic("http_alipay_app_pay")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "alipay_not_configured",
			"message": "服务端未配置 ALIPAY_* 或 PUBLIC_BASE_URL",
		})
		return
	}
	oid := config.ParseOrderIDParam(r.PathValue("id"))
	if oid <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_order"})
		return
	}
	ctx := r.Context()
	o, err := s.Store.GetOrderByID(ctx, oid)
	if err != nil || o.UserID != uid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "order_not_found"})
		return
	}
	if o.Status != "pending" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "order_not_payable"})
		return
	}
	cli, err := alipayapp.NewClient(s.Cfg)
	if err != nil {
		log.Printf("alipay app-pay init: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "alipay_init_failed",
			"message": err.Error(),
		})
		return
	}
	param := alipay.TradeAppPay{}
	param.NotifyURL = s.alipayNotifyAbsURL()
	param.Subject = alipaySubjectForPlan(o.PlanID)
	param.OutTradeNo = o.OutTradeNo
	param.TotalAmount = amountFenToAlipayYuan(o.AmountTotal)
	param.ProductCode = "QUICK_MSECURITY_PAY"
	param.Body = "qingyu_cloud"

	orderStr, err := cli.TradeAppPay(param)
	if err != nil {
		log.Printf("alipay trade app pay: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   "alipay_sign_failed",
			"message": err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"order_str": orderStr,
		"app_id":    strings.TrimSpace(s.Cfg.AlipayAppID),
	})
}

// handleAlipayPagePaySign 为待支付订单生成 alipay.trade.page.pay 收银台 URL（桌面浏览器打开）。
// 须与「App 支付」区分：开放平台须单独签约并生效「电脑网站支付」，否则收银台页常见 insufficient-isv-permissions。
func (s *Server) handleAlipayPagePaySign(w http.ResponseWriter, r *http.Request, uid int64) {
	if !s.Cfg.AlipayPcPayConfigured() {
		s.Cfg.LogAlipayDiagnostic("http_alipay_page_pay")
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "alipay_not_configured",
			"message": "服务端未配置 ALIPAY_PC_* 或 PUBLIC_BASE_URL",
		})
		return
	}
	oid := config.ParseOrderIDParam(r.PathValue("id"))
	if oid <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_order"})
		return
	}
	ctx := r.Context()
	o, err := s.Store.GetOrderByID(ctx, oid)
	if err != nil || o.UserID != uid {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "order_not_found"})
		return
	}
	if o.Status != "pending" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "order_not_payable"})
		return
	}
	cli, err := alipayapp.NewPcClient(s.Cfg)
	if err != nil {
		log.Printf("alipay page-pay init: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error":   "alipay_init_failed",
			"message": err.Error(),
		})
		return
	}
	ret := strings.TrimSpace(s.Cfg.AlipayPagePayReturnURL)
	if ret == "" {
		ret = "https://note.kymjs.com/private/harmony.html"
	}
	param := alipay.TradePagePay{}
	param.NotifyURL = s.alipayNotifyAbsURL()
	param.ReturnURL = ret
	param.Subject = alipaySubjectForPlan(o.PlanID)
	param.OutTradeNo = o.OutTradeNo
	param.TotalAmount = amountFenToAlipayYuan(o.AmountTotal)
	param.ProductCode = "FAST_INSTANT_TRADE_PAY"
	param.Body = "qingyu_cloud"

	payURL, err := cli.TradePagePay(param)
	if err != nil {
		log.Printf("alipay trade page pay: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error":   "alipay_page_pay_failed",
			"message": err.Error(),
		})
		return
	}
	if payURL == nil || strings.TrimSpace(payURL.String()) == "" {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "alipay_page_pay_empty",
		})
		return
	}
	u := payURL.String()
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		log.Printf("alipay page pay invalid url err=%v scheme=%q host=%q", err, parsed.Scheme, parsed.Host)
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": "alipay_page_pay_invalid_url",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"pay_url": u,
		"app_id":  strings.TrimSpace(s.Cfg.AlipayPcAppID),
	})
}

// handleAlipayNotify 处理支付宝异步通知（application/x-www-form-urlencoded），验签成功后标记订单并顺延订阅。
func (s *Server) handleAlipayNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := r.ParseForm(); err != nil {
		log.Printf("alipay notify parse: %v", err)
		_, _ = w.Write([]byte("fail"))
		return
	}
	hasMobile := s.Cfg.AlipayCoreConfigured()
	hasPc := s.Cfg.AlipayPcCoreConfigured()
	if !hasMobile && !hasPc {
		s.Cfg.LogAlipayDiagnostic("http_alipay_notify_core_off")
		log.Printf("alipay notify: no alipay configured")
		_, _ = w.Write([]byte("fail"))
		return
	}

	// 优先用移动端客户端验签；若 APP ID 不匹配再尝试 PC 端。
	var cli *alipay.Client
	var err error
	if hasMobile {
		cli, err = alipayapp.NewClient(s.Cfg)
		if err == nil {
			n, _ := cli.DecodeNotification(r.Form)
			if n != nil && (strings.TrimSpace(n.AppId) == strings.TrimSpace(s.Cfg.AlipayAppID)) {
				s.handleAlipayNotifyValid(w, r, cli, n)
				return
			}
		}
	}
	if hasPc {
		cli, err = alipayapp.NewPcClient(s.Cfg)
		if err == nil {
			n, _ := cli.DecodeNotification(r.Form)
			if n != nil && (strings.TrimSpace(n.AppId) == strings.TrimSpace(s.Cfg.AlipayPcAppID)) {
				s.handleAlipayNotifyValid(w, r, cli, n)
				return
			}
		}
	}
	log.Printf("alipay notify: no client matched app_id")
	_, _ = w.Write([]byte("fail"))
}

func (s *Server) handleAlipayNotifyValid(w http.ResponseWriter, r *http.Request, cli *alipay.Client, n *alipay.Notification) {
	ctx := r.Context()
	outNo := strings.TrimSpace(n.OutTradeNo)
	if outNo == "" {
		_, _ = w.Write([]byte("fail"))
		return
	}
	o, err := s.Store.GetOrderByOutTradeNo(ctx, outNo)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			log.Printf("alipay notify unknown out_trade_no %s", outNo)
			alipay.ACKNotification(w)
			return
		}
		log.Printf("alipay notify db: %v", err)
		_, _ = w.Write([]byte("fail"))
		return
	}

	tradeNo := strings.TrimSpace(n.TradeNo)
	if tradeNo == "" {
		_, _ = w.Write([]byte("fail"))
		return
	}

	if o.Status == "paid" {
		exist := ""
		if o.TransactionID.Valid {
			exist = strings.TrimSpace(o.TransactionID.String)
		}
		if exist == "" || exist == tradeNo {
			alipay.ACKNotification(w)
			return
		}
		log.Printf("alipay notify paid conflict out=%s old_tx=%s new_tx=%s", outNo, exist, tradeNo)
		alipay.ACKNotification(w)
		return
	}

	switch n.TradeStatus {
	case alipay.TradeStatusSuccess, alipay.TradeStatusFinished:
	default:
		alipay.ACKNotification(w)
		return
	}

	gotFen, ok := parseAlipayTotalFen(n.TotalAmount)
	if !ok || gotFen != o.AmountTotal {
		log.Printf("alipay notify amount mismatch out=%s want_fen=%d got=%q", outNo, o.AmountTotal, n.TotalAmount)
		_, _ = w.Write([]byte("fail"))
		return
	}

	prev, errLookup := s.Store.GetOrderByTransactionID(ctx, tradeNo)
	if errLookup != nil && !errors.Is(errLookup, sql.ErrNoRows) {
		log.Printf("alipay notify lookup trade_no: %v", errLookup)
		_, _ = w.Write([]byte("fail"))
		return
	}
	if errLookup == nil && prev != nil && prev.ID != o.ID {
		log.Printf("alipay notify duplicate trade_no different order")
		_, _ = w.Write([]byte("fail"))
		return
	}

	if err := s.Store.MarkOrderPaid(ctx, o.OutTradeNo, tradeNo); err != nil {
		refreshed, err2 := s.Store.GetOrderByOutTradeNo(ctx, outNo)
		if err2 == nil && refreshed.Status == "paid" &&
			refreshed.TransactionID.Valid &&
			strings.TrimSpace(refreshed.TransactionID.String) == tradeNo {
			alipay.ACKNotification(w)
			return
		}
		log.Printf("alipay mark paid: %v", err)
		_, _ = w.Write([]byte("fail"))
		return
	}
	s.extendQingyuSubscriptionAfterPayment(ctx, o.UserID, o.PlanID, &store.MembershipRechargeRecordParams{
		UserID:               o.UserID,
		Channel:              "alipay",
		OrderID:              sql.NullInt64{Int64: o.ID, Valid: true},
		OutTradeNo:           sql.NullString{String: o.OutTradeNo, Valid: true},
		GatewayTransactionID: sql.NullString{String: tradeNo, Valid: true},
		PlanID:               o.PlanID,
	})
	alipay.ACKNotification(w)
}
