// 在服务器上签发兑换码：需环境变量 MYSQL_DSN、REDEMPTION_ISSUE_SECRET（与命令行 -issuer-secret 一致）、
// FEISHU_REDEMPTION_WEBHOOK_URL（可选；设置则每张码单独 POST 到飞书机器人）。
//
// 示例（部署目录已 export 或 source .env）：
//
//	go run ./cmd/issue_redemption_codes -plan monthly -count 5 -issuer-secret "$REDEMPTION_ISSUE_SECRET"
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kymjs/noteapi/internal/redemption"
	"github.com/kymjs/noteapi/internal/store"
)

func main() {
	plan := flag.String("plan", "", "monthly | half_year | yearly | lifetime_vip")
	count := flag.Int("count", 0, "生成条数，>=1")
	issuerSecret := flag.String("issuer-secret", "", "须与环境变量 REDEMPTION_ISSUE_SECRET 完全一致")
	flag.Parse()

	if !validPlan(*plan) || *count < 1 {
		flag.Usage()
		os.Exit(2)
	}
	envSecret := strings.TrimSpace(os.Getenv("REDEMPTION_ISSUE_SECRET"))
	if envSecret == "" || *issuerSecret == "" || envSecret != *issuerSecret {
		log.Fatal("REDEMPTION_ISSUE_SECRET 未设置或与 -issuer-secret 不一致")
	}

	dsn := strings.TrimSpace(os.Getenv("MYSQL_DSN"))
	st, err := store.OpenMySQL(dsn)
	if err != nil {
		log.Fatalf("mysql: %v", err)
	}
	defer st.DB.Close()

	ctx := context.Background()
	webhook := strings.TrimSpace(os.Getenv("FEISHU_REDEMPTION_WEBHOOK_URL"))

	for i := 0; i < *count; i++ {
		plain, hash, errGen := newRandomCode()
		if errGen != nil {
			log.Fatalf("rand: %v", errGen)
		}
		if err := st.InsertRedemptionCode(ctx, *plan, hash); err != nil {
			log.Fatalf("insert: %v", err)
		}
		fmt.Println(plain)
		if webhook != "" {
			if err := postFeishuText(webhook, feishuBody(*plan, plain)); err != nil {
				log.Printf("warning: feishu webhook failed for code ending …%s: %v", plain[len(plain)-4:], err)
			}
		}
	}
}

func validPlan(p string) bool {
	switch strings.TrimSpace(p) {
	case "monthly", "half_year", "yearly", "lifetime_vip":
		return true
	default:
		return false
	}
}

func newRandomCode() (plain, hash string, err error) {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain = "QY-" + strings.ToUpper(hex.EncodeToString(b))
	h := redemption.HashNormalized(redemption.NormalizeCode(plain))
	return plain, h, nil
}

func planTitleCN(plan string) string {
	switch plan {
	case "monthly":
		return "月卡"
	case "half_year":
		return "半年卡"
	case "yearly":
		return "年卡"
	case "lifetime_vip":
		return "终身 VIP"
	default:
		return plan
	}
}

func feishuBody(plan, code string) string {
	return fmt.Sprintf("【轻羽云兑换码】\n类型：%s（%s）\n兑换码：%s\n（仅限一次性使用，请勿转发给无关人员）",
		planTitleCN(plan), plan, code)
}

type feishuTextPayload struct {
	MsgType string `json:"msg_type"`
	Content struct {
		Text string `json:"text"`
	} `json:"content"`
}

func postFeishuText(webhookURL, text string) error {
	var p feishuTextPayload
	p.MsgType = "text"
	p.Content.Text = text
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	return nil
}
