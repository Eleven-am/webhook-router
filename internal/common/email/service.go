package email

import (
	"crypto/tls"
	"fmt"
	"net/smtp"
	"strconv"
	"strings"

	"webhook-router/internal/common/logging"
	"webhook-router/internal/config"
)

// Service provides email sending functionality
type Service struct {
	config *config.Config
	logger logging.Logger
}

// NewService creates a new email service
func NewService(cfg *config.Config, logger logging.Logger) *Service {
	return &Service{
		config: cfg,
		logger: logger,
	}
}

// SendPasswordResetEmail sends a password reset email to the user
func (s *Service) SendPasswordResetEmail(toEmail, resetToken string) error {
	if !s.config.SMTPEnabled {
		s.logger.Warn("SMTP is not enabled, skipping email send",
			logging.Field{"to", toEmail},
			logging.Field{"reason", "SMTP_ENABLED=false"})
		return nil
	}

	// Construct reset link using configurable base URL
	resetLink := fmt.Sprintf("%s/reset-password?token=%s", s.config.FrontendBaseURL, resetToken)

	subject := "Password Reset Request"
	body := fmt.Sprintf(`
Hello,

You have requested to reset your password for Webhook Router.

Please click the link below to reset your password:
%s

This link will expire in 1 hour.

If you did not request this password reset, please ignore this email.

Best regards,
The Webhook Router Team
`, resetLink)

	htmlBody := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <style>
        body { font-family: Arial, sans-serif; background-color: #f4f4f4; padding: 20px; }
        .container { max-width: 600px; margin: 0 auto; background-color: white; padding: 30px; border-radius: 10px; box-shadow: 0 2px 5px rgba(0,0,0,0.1); }
        .header { color: #333; margin-bottom: 20px; }
        .button { display: inline-block; padding: 12px 24px; background-color: #7c3aed; color: white; text-decoration: none; border-radius: 5px; margin: 20px 0; }
        .footer { color: #666; font-size: 14px; margin-top: 30px; }
    </style>
</head>
<body>
    <div class="container">
        <h2 class="header">Password Reset Request</h2>
        <p>Hello,</p>
        <p>You have requested to reset your password for Webhook Router.</p>
        <p>Please click the button below to reset your password:</p>
        <a href="%s" class="button">Reset Password</a>
        <p>Or copy and paste this link into your browser:</p>
        <p><code>%s</code></p>
        <p>This link will expire in 1 hour.</p>
        <p class="footer">If you did not request this password reset, please ignore this email.</p>
        <p class="footer">Best regards,<br>The Webhook Router Team</p>
    </div>
</body>
</html>
`, resetLink, resetLink)

	return s.SendEmail(toEmail, subject, body, htmlBody)
}

// SendEmail sends an email with both plain text and HTML content
func (s *Service) SendEmail(to, subject, plainBody, htmlBody string) error {
	if !s.config.SMTPEnabled {
		return fmt.Errorf("SMTP is not enabled")
	}

	from := s.config.SMTPFrom
	if s.config.SMTPFromName != "" {
		from = fmt.Sprintf("%s <%s>", s.config.SMTPFromName, s.config.SMTPFrom)
	}

	// Construct the email
	headers := make(map[string]string)
	headers["From"] = from
	headers["To"] = to
	headers["Subject"] = subject
	headers["MIME-Version"] = "1.0"
	headers["Content-Type"] = "multipart/alternative; boundary=\"boundary123\""

	message := ""
	for k, v := range headers {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n"

	// Plain text part
	message += "--boundary123\r\n"
	message += "Content-Type: text/plain; charset=\"UTF-8\"\r\n"
	message += "\r\n"
	message += plainBody + "\r\n"

	// HTML part
	message += "--boundary123\r\n"
	message += "Content-Type: text/html; charset=\"UTF-8\"\r\n"
	message += "\r\n"
	message += htmlBody + "\r\n"
	message += "--boundary123--"

	// Send the email
	auth := smtp.PlainAuth("", s.config.SMTPUsername, s.config.SMTPPassword, s.config.SMTPHost)

	addr := fmt.Sprintf("%s:%s", s.config.SMTPHost, s.config.SMTPPort)

	// Handle TLS/SSL configuration
	if s.config.SMTPUseSSL {
		// Use implicit TLS
		return s.sendEmailWithSSL(addr, auth, s.config.SMTPFrom, []string{to}, []byte(message))
	} else if s.config.SMTPUseTLS {
		// Use STARTTLS
		return smtp.SendMail(addr, auth, s.config.SMTPFrom, []string{to}, []byte(message))
	} else {
		// Plain SMTP (not recommended)
		return smtp.SendMail(addr, auth, s.config.SMTPFrom, []string{to}, []byte(message))
	}
}

// sendEmailWithSSL sends email using implicit TLS/SSL
func (s *Service) sendEmailWithSSL(addr string, auth smtp.Auth, from string, to []string, msg []byte) error {
	host := s.config.SMTPHost
	port, _ := strconv.Atoi(s.config.SMTPPort)

	tlsConfig := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: s.config.SMTPSkipVerify,
	}

	conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", host, port), tlsConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer client.Close()

	if auth != nil {
		if err = client.Auth(auth); err != nil {
			return fmt.Errorf("SMTP authentication failed: %w", err)
		}
	}

	if err = client.Mail(from); err != nil {
		return err
	}

	for _, addr := range to {
		if err = client.Rcpt(addr); err != nil {
			return err
		}
	}

	w, err := client.Data()
	if err != nil {
		return err
	}

	_, err = w.Write(msg)
	if err != nil {
		return err
	}

	err = w.Close()
	if err != nil {
		return err
	}

	return client.Quit()
}

// ValidateEmailAddress performs basic email validation
func ValidateEmailAddress(email string) bool {
	// Basic validation - check for @ and domain
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return false
	}

	// Check local part is not empty
	if len(parts[0]) == 0 {
		return false
	}

	// Check domain has at least one dot
	if !strings.Contains(parts[1], ".") {
		return false
	}

	return true
}
