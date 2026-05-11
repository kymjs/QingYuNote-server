package aliyunsms

import (
	"fmt"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/dypnsapi"
)

// SMSParams 与控制台赠送签名/模板一致；TemplateParam 含 ##code## 时须配合 Send 里的 CodeType。
type SMSParams struct {
	SignName     string
	TemplateCode string
	SchemeName   string
	// TemplateParam JSON，例如 {"code":"##code##","min":"5"}
	TemplateParam string
}

func NewClient(region, ak, sk string) (*dypnsapi.Client, error) {
	r := strings.TrimSpace(region)
	if r == "" {
		r = "cn-hangzhou"
	}
	return dypnsapi.NewClientWithAccessKey(r, strings.TrimSpace(ak), strings.TrimSpace(sk))
}

// SendVerifyCode 调用 SendSmsVerifyCode，下发短信验证码（服务端生成验证码并可由 Check 核验）。
func SendVerifyCode(cli *dypnsapi.Client, p SMSParams, phoneDigits string) error {
	req := dypnsapi.CreateSendSmsVerifyCodeRequest()
	req.PhoneNumber = strings.TrimSpace(phoneDigits)
	req.SignName = strings.TrimSpace(p.SignName)
	req.TemplateCode = strings.TrimSpace(p.TemplateCode)
	param := strings.TrimSpace(p.TemplateParam)
	if param == "" {
		param = `{"code":"##code##","min":"5"}`
	}
	req.TemplateParam = param
	req.CodeType = requests.NewInteger(1)
	req.Interval = requests.NewInteger(60)
	if sn := strings.TrimSpace(p.SchemeName); sn != "" {
		req.SchemeName = sn
	}
	resp, err := cli.SendSmsVerifyCode(req)
	if err != nil {
		return fmt.Errorf("%w", err)
	}
	if resp == nil {
		return fmt.Errorf("empty response")
	}
	if strings.EqualFold(resp.Code, "OK") && resp.Success {
		return nil
	}
	code := strings.TrimSpace(resp.Code)
	msg := strings.TrimSpace(resp.Message)
	if code != "" {
		return fmt.Errorf("aliyun:%s:%s", code, msg)
	}
	return fmt.Errorf("aliyun:%s", msg)
}

// CheckVerifyCode 调用 CheckSmsVerifyCode；返回是否核验通过（VerifyResult==PASS）。
func CheckVerifyCode(cli *dypnsapi.Client, p SMSParams, phoneDigits, verifyCode string) (bool, error) {
	req := dypnsapi.CreateCheckSmsVerifyCodeRequest()
	req.PhoneNumber = strings.TrimSpace(phoneDigits)
	req.VerifyCode = strings.TrimSpace(verifyCode)
	if sn := strings.TrimSpace(p.SchemeName); sn != "" {
		req.SchemeName = sn
	}
	resp, err := cli.CheckSmsVerifyCode(req)
	if err != nil {
		return false, fmt.Errorf("%w", err)
	}
	if resp == nil {
		return false, fmt.Errorf("empty response")
	}
	if strings.EqualFold(resp.Code, "OK") && resp.Success {
		return strings.EqualFold(strings.TrimSpace(resp.Model.VerifyResult), "PASS"), nil
	}
	return false, fmt.Errorf("aliyun:%s:%s", strings.TrimSpace(resp.Code), strings.TrimSpace(resp.Message))
}
