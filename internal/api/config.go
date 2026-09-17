package api

import (
	"encoding/base64"
	"net/url"
	"strings"
)

type Config struct{ BaseURL, Email, Token string }

// Load supports separate sites/credentials per product and scoped API tokens.
// Credentials are read from the environment and are never persisted.
func Load(getenv func(string) string, product string) (Config, error) {
	if product != "jira" && product != "confluence" {
		return Config{}, &Error{Code: "configuration", Message: "Unknown Atlassian product."}
	}
	prefix := strings.ToUpper(product)
	first := func(names ...string) string {
		for _, n := range names {
			if v := strings.TrimSpace(getenv(n)); v != "" {
				return v
			}
		}
		return ""
	}
	c := Config{
		BaseURL: first(prefix+"_URL", "ATLASSIAN_URL"),
		Email:   first(prefix+"_EMAIL", prefix+"_USER", "ATLASSIAN_EMAIL"),
		Token:   first(prefix+"_API_TOKEN", prefix+"_TOKEN", "ATLASSIAN_API_TOKEN"),
	}
	if c.BaseURL == "" || c.Email == "" || c.Token == "" {
		return c, &Error{Code: "configuration", Message: "Set ATLASSIAN_URL, ATLASSIAN_EMAIL, ATLASSIAN_API_TOKEN (or product-specific equivalents)."}
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(c.BaseURL, "#") {
		return c, &Error{Code: "configuration", Message: "Site URL must be an HTTPS URL without credentials, query, or fragment."}
	}
	if u.Path != "" && u.Path != "/" && u.Path != "/wiki" && u.Path != "/wiki/" {
		return c, &Error{Code: "configuration", Message: "Use the site root URL; set ATLASSIAN_CLOUD_ID for scoped tokens."}
	}
	u.Path, u.RawPath = "", ""
	c.BaseURL = u.String()
	cloudID := first(prefix+"_CLOUD_ID", "ATLASSIAN_CLOUD_ID")
	if cloudID != "" {
		for _, ch := range cloudID {
			if !strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-", ch) {
				return c, &Error{Code: "configuration", Message: "Cloud ID must contain only letters, digits, and hyphens."}
			}
		}
		c.BaseURL = "https://api.atlassian.com/ex/" + product + "/" + cloudID
	}
	return c, nil
}

func (c Config) secrets() []string {
	return []string{c.Token, base64.StdEncoding.EncodeToString([]byte(c.Email + ":" + c.Token))}
}

// DiagnosticSecrets includes aliases and trimmed values actually used by Load.
func DiagnosticSecrets(getenv func(string) string) []string {
	var tokens, emails []string
	for _, name := range []string{"ATLASSIAN_API_TOKEN", "JIRA_API_TOKEN", "JIRA_TOKEN", "CONFLUENCE_API_TOKEN", "CONFLUENCE_TOKEN"} {
		if value := getenv(name); value != "" {
			tokens = append(tokens, value, strings.TrimSpace(value))
		}
	}
	for _, name := range []string{"ATLASSIAN_EMAIL", "JIRA_EMAIL", "JIRA_USER", "CONFLUENCE_EMAIL", "CONFLUENCE_USER"} {
		if value := strings.TrimSpace(getenv(name)); value != "" {
			emails = append(emails, value)
		}
	}
	secrets := append([]string(nil), tokens...)
	for _, email := range emails {
		for _, token := range tokens {
			secrets = append(secrets, base64.StdEncoding.EncodeToString([]byte(email+":"+token)))
		}
	}
	return secrets
}
