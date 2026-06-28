package handlers

import (
	"bytes"
	"encoding/json"
	"feedback/internal/db"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

type ResendPayload struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

func sendEmailNotification(submissionId int64, formName, notifyEmail string, data SubmitRequest, clientIP string) {
	apiKey := os.Getenv("RESEND_API_KEY")
	if apiKey == "" {
		log.Println("RESEND_API_KEY not configured, skipping email")
		return
	}

	fromEmail := os.Getenv("FROM_EMAIL")
	if fromEmail == "" {
		fromEmail = "hello@form.542500.xyz"
	}
	fromName := os.Getenv("FROM_NAME")
	if fromName == "" {
		fromName = "Form Messenger"
	}

	// i18n subject and labels
	lang := data.Lang
	var subject string
	var labelName, labelEmail, labelPhone, labelContent, labelSource, labelIP string
	if lang == "zh" {
		subject = fmt.Sprintf("来自 %s 的反馈", formName)
		labelName = "姓名"
		labelEmail = "邮箱"
		labelPhone = "电话"
		labelContent = "内容"
		labelSource = "来源"
		labelIP = "IP"
	} else {
		subject = fmt.Sprintf("Feedback from %s", formName)
		labelName = "Name"
		labelEmail = "Email"
		labelPhone = "Phone"
		labelContent = "Content"
		labelSource = "Source"
		labelIP = "IP"
	}

	body := fmt.Sprintf(
		"%s: %s\n%s: %s\n%s: %s\n\n%s: %s\n\n---\n%s: %s\n%s: %s",
		labelName, data.Name,
		labelEmail, data.Email,
		labelPhone, data.Phone,
		labelContent, data.Content,
		labelSource, data.SourceURL,
		labelIP, clientIP,
	)

	payload := ResendPayload{
		From:    fmt.Sprintf("%s <%s>", fromName, fromEmail),
		To:      []string{notifyEmail},
		Subject: subject,
		Text:    body,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		log.Printf("Failed to marshal Resend payload: %v", err)
		updateResendStatus(submissionId, "error: marshal failed")
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}

	proxyUrlStr := os.Getenv("HTTP_PROXY")
	if proxyUrlStr != "" {
		proxyUrl, err := url.Parse(proxyUrlStr)
		if err == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyUrl)}
		}
	}

	req, err := http.NewRequest("POST", "https://api.resend.com/emails", bytes.NewReader(jsonData))
	if err != nil {
		log.Printf("Failed to create Resend request: %v", err)
		updateResendStatus(submissionId, "error: request creation failed")
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Resend API call failed: %v", err)
		updateResendStatus(submissionId, "error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		updateResendStatus(submissionId, "sent")
		log.Printf("Email sent to %s for submission #%d", notifyEmail, submissionId)
	} else {
		var resBody map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&resBody)
		log.Printf("Resend API returned %d: %v", resp.StatusCode, resBody)
		updateResendStatus(submissionId, "error: HTTP "+resp.Status)
	}
}

func updateResendStatus(submissionId int64, status string) {
	_, err := db.DB.Exec("UPDATE submissions SET resend_status = ? WHERE id = ?", status, submissionId)
	if err != nil {
		log.Printf("Failed to update resend_status for submission #%d: %v", submissionId, err)
	}
}
