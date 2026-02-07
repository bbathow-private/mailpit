package bouncerules

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// buildMessage creates a minimal RFC 5322 message for testing
func buildMessage(from, to, subject, body string, extraHeaders map[string]string) []byte {
	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n", from, to, subject)
	for k, v := range extraHeaders {
		msg += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	msg += "\r\n" + body
	return []byte(msg)
}

func resetState() {
	mu.Lock()
	rules = []Rule{}
	mu.Unlock()
	Enabled = true
}

func TestMatchSubject(t *testing.T) {
	resetState()

	if err := SetRules([]Rule{
		{Field: FieldSubject, Pattern: "BOUNCE-HARD", Action: ActionHardBounce},
	}); err != nil {
		t.Fatal(err)
	}

	data := buildMessage("sender@example.com", "rcpt@example.com", "Please BOUNCE-HARD this", "Hello", nil)

	rule, matched := Evaluate("sender@example.com", []string{"rcpt@example.com"}, data)
	if !matched {
		t.Fatal("expected rule to match on subject")
	}
	if rule.Action != ActionHardBounce {
		t.Fatalf("expected hard_bounce action, got %s", rule.Action)
	}
}

func TestMatchFrom(t *testing.T) {
	resetState()

	if err := SetRules([]Rule{
		{Field: FieldFrom, Pattern: "bounce@", Action: ActionSoftBounce},
	}); err != nil {
		t.Fatal(err)
	}

	data := buildMessage("bounce@example.com", "rcpt@example.com", "Hello", "Body", nil)

	_, matched := Evaluate("bounce@example.com", []string{"rcpt@example.com"}, data)
	if !matched {
		t.Fatal("expected rule to match on from")
	}
}

func TestMatchTo(t *testing.T) {
	resetState()

	if err := SetRules([]Rule{
		{Field: FieldTo, Pattern: "hard-bounce@", Action: ActionHardBounce},
	}); err != nil {
		t.Fatal(err)
	}

	data := buildMessage("sender@example.com", "hard-bounce@example.com", "Hello", "Body", nil)

	_, matched := Evaluate("sender@example.com", []string{"hard-bounce@example.com"}, data)
	if !matched {
		t.Fatal("expected rule to match on to")
	}
}

func TestMatchBody(t *testing.T) {
	resetState()

	if err := SetRules([]Rule{
		{Field: FieldBody, Pattern: "trigger-mailbox-full", Action: ActionQuotaFull},
	}); err != nil {
		t.Fatal(err)
	}

	data := buildMessage("sender@example.com", "rcpt@example.com", "Hello", "Please trigger-mailbox-full now", nil)

	rule, matched := Evaluate("sender@example.com", []string{"rcpt@example.com"}, data)
	if !matched {
		t.Fatal("expected rule to match on body")
	}
	if rule.Action != ActionQuotaFull {
		t.Fatalf("expected quota_full action, got %s", rule.Action)
	}
}

func TestMatchHeader(t *testing.T) {
	resetState()

	if err := SetRules([]Rule{
		{Field: FieldHeader, HeaderName: "X-Test-Bounce", Pattern: "^reject$", Action: ActionReject},
	}); err != nil {
		t.Fatal(err)
	}

	data := buildMessage("sender@example.com", "rcpt@example.com", "Hello", "Body",
		map[string]string{"X-Test-Bounce": "reject"})

	rule, matched := Evaluate("sender@example.com", []string{"rcpt@example.com"}, data)
	if !matched {
		t.Fatal("expected rule to match on header")
	}
	if rule.Action != ActionReject {
		t.Fatalf("expected reject action, got %s", rule.Action)
	}
}

func TestNoMatch(t *testing.T) {
	resetState()

	if err := SetRules([]Rule{
		{Field: FieldSubject, Pattern: "BOUNCE-HARD", Action: ActionHardBounce},
	}); err != nil {
		t.Fatal(err)
	}

	data := buildMessage("sender@example.com", "rcpt@example.com", "Normal email", "Body", nil)

	_, matched := Evaluate("sender@example.com", []string{"rcpt@example.com"}, data)
	if matched {
		t.Fatal("expected no match")
	}
}

func TestDisabledReturnsNoMatch(t *testing.T) {
	resetState()
	Enabled = false

	if err := SetRules([]Rule{
		{Field: FieldSubject, Pattern: "BOUNCE", Action: ActionHardBounce},
	}); err != nil {
		t.Fatal(err)
	}

	data := buildMessage("sender@example.com", "rcpt@example.com", "BOUNCE", "Body", nil)

	_, matched := Evaluate("sender@example.com", []string{"rcpt@example.com"}, data)
	if matched {
		t.Fatal("expected no match when disabled")
	}
}

func TestEmptyRulesReturnsNoMatch(t *testing.T) {
	resetState()

	data := buildMessage("sender@example.com", "rcpt@example.com", "BOUNCE", "Body", nil)

	_, matched := Evaluate("sender@example.com", []string{"rcpt@example.com"}, data)
	if matched {
		t.Fatal("expected no match with empty rules")
	}
}

func TestPriorityOrdering(t *testing.T) {
	resetState()

	if err := SetRules([]Rule{
		{Field: FieldSubject, Pattern: "BOUNCE", Action: ActionSoftBounce, Priority: 10},
		{Field: FieldSubject, Pattern: "BOUNCE", Action: ActionHardBounce, Priority: 1},
	}); err != nil {
		t.Fatal(err)
	}

	data := buildMessage("sender@example.com", "rcpt@example.com", "BOUNCE", "Body", nil)

	rule, matched := Evaluate("sender@example.com", []string{"rcpt@example.com"}, data)
	if !matched {
		t.Fatal("expected match")
	}
	if rule.Action != ActionHardBounce {
		t.Fatalf("expected hard_bounce (priority 1) to match first, got %s", rule.Action)
	}
}

func TestCustomErrorCodeAndMessage(t *testing.T) {
	resetState()

	rule := Rule{
		Field:        FieldSubject,
		Pattern:      "BOUNCE",
		Action:       ActionHardBounce,
		ErrorCode:    553,
		ErrorMessage: "5.1.3 Invalid address format",
	}
	if err := AddRule(rule); err != nil {
		t.Fatal(err)
	}

	data := buildMessage("sender@example.com", "rcpt@example.com", "BOUNCE", "Body", nil)

	matched, ok := Evaluate("sender@example.com", []string{"rcpt@example.com"}, data)
	if !ok {
		t.Fatal("expected match")
	}

	code, msg := matched.SMTPResponse()
	if code != 553 {
		t.Fatalf("expected code 553, got %d", code)
	}
	if msg != "5.1.3 Invalid address format" {
		t.Fatalf("expected custom message, got %q", msg)
	}
}

func TestDefaultSMTPResponse(t *testing.T) {
	tests := []struct {
		action       Action
		expectedCode int
		expectedMsg  string
	}{
		{ActionHardBounce, 550, "5.1.1 User unknown"},
		{ActionSoftBounce, 451, "4.7.1 Try again later"},
		{ActionReject, 554, "5.7.1 Message rejected"},
		{ActionQuotaFull, 452, "4.2.2 Mailbox full"},
	}

	for _, tt := range tests {
		rule := Rule{Action: tt.action}
		code, msg := rule.SMTPResponse()
		if code != tt.expectedCode {
			t.Errorf("action %s: expected code %d, got %d", tt.action, tt.expectedCode, code)
		}
		if msg != tt.expectedMsg {
			t.Errorf("action %s: expected msg %q, got %q", tt.action, tt.expectedMsg, msg)
		}
	}
}

func TestInvalidRegexReturnsError(t *testing.T) {
	resetState()

	err := SetRules([]Rule{
		{Field: FieldSubject, Pattern: "[invalid", Action: ActionHardBounce},
	})
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestInvalidFieldReturnsError(t *testing.T) {
	resetState()

	err := SetRules([]Rule{
		{Field: "invalid", Pattern: "test", Action: ActionHardBounce},
	})
	if err == nil {
		t.Fatal("expected error for invalid field")
	}
}

func TestInvalidActionReturnsError(t *testing.T) {
	resetState()

	err := SetRules([]Rule{
		{Field: FieldSubject, Pattern: "test", Action: "invalid"},
	})
	if err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func TestHeaderFieldRequiresHeaderName(t *testing.T) {
	resetState()

	err := SetRules([]Rule{
		{Field: FieldHeader, Pattern: "test", Action: ActionHardBounce},
	})
	if err == nil {
		t.Fatal("expected error when header field has no header_name")
	}
}

func TestInvalidErrorCode(t *testing.T) {
	resetState()

	err := SetRules([]Rule{
		{Field: FieldSubject, Pattern: "test", Action: ActionHardBounce, ErrorCode: 200},
	})
	if err == nil {
		t.Fatal("expected error for error_code outside 400-599")
	}
}

func TestAddAndDeleteRule(t *testing.T) {
	resetState()

	if err := AddRule(Rule{ID: "rule1", Field: FieldSubject, Pattern: "test", Action: ActionHardBounce}); err != nil {
		t.Fatal(err)
	}

	rules := GetRules()
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	if err := DeleteRule("rule1"); err != nil {
		t.Fatal(err)
	}

	rules = GetRules()
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules after delete, got %d", len(rules))
	}
}

func TestDeleteNonExistentRule(t *testing.T) {
	resetState()

	err := DeleteRule("nonexistent")
	if err == nil {
		t.Fatal("expected error deleting nonexistent rule")
	}
}

func TestClearRules(t *testing.T) {
	resetState()

	if err := SetRules([]Rule{
		{Field: FieldSubject, Pattern: "test1", Action: ActionHardBounce},
		{Field: FieldSubject, Pattern: "test2", Action: ActionSoftBounce},
	}); err != nil {
		t.Fatal(err)
	}

	ClearRules()

	rules := GetRules()
	if len(rules) != 0 {
		t.Fatalf("expected 0 rules after clear, got %d", len(rules))
	}
}

func TestAutoGenerateID(t *testing.T) {
	resetState()

	if err := AddRule(Rule{Field: FieldSubject, Pattern: "test", Action: ActionHardBounce}); err != nil {
		t.Fatal(err)
	}

	rules := GetRules()
	if rules[0].ID == "" {
		t.Fatal("expected auto-generated ID")
	}
}

func TestLoadFromYAML(t *testing.T) {
	resetState()

	yamlContent := `rules:
  - field: subject
    pattern: "BOUNCE-HARD"
    action: hard_bounce
    priority: 10
  - field: to
    pattern: "hard-bounce@"
    action: hard_bounce
    priority: 1
  - field: header
    header_name: X-Test-Bounce
    pattern: "^quota$"
    action: quota_full
    priority: 5
`

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "bounce-rules.yaml")
	if err := os.WriteFile(tmpFile, []byte(yamlContent), 0644); err != nil {
		t.Fatal(err)
	}

	if err := LoadFromYAML(tmpFile); err != nil {
		t.Fatal(err)
	}

	rules := GetRules()
	if len(rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(rules))
	}

	// priority 1 should be first after sorting
	if rules[0].Field != FieldTo {
		t.Fatalf("expected first rule to be 'to' (priority 1), got %s", rules[0].Field)
	}
}

func TestLoadFromYAMLInvalidFile(t *testing.T) {
	resetState()

	err := LoadFromYAML("/nonexistent/file.yaml")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestLoadFromYAMLInvalidContent(t *testing.T) {
	resetState()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "bad.yaml")
	if err := os.WriteFile(tmpFile, []byte("invalid: [yaml: content"), 0644); err != nil {
		t.Fatal(err)
	}

	err := LoadFromYAML(tmpFile)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestConcurrentReadWrite(t *testing.T) {
	resetState()

	if err := SetRules([]Rule{
		{Field: FieldSubject, Pattern: "BOUNCE", Action: ActionHardBounce},
	}); err != nil {
		t.Fatal(err)
	}

	data := buildMessage("sender@example.com", "rcpt@example.com", "BOUNCE", "Body", nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			Evaluate("sender@example.com", []string{"rcpt@example.com"}, data)
		}()
		go func(i int) {
			defer wg.Done()
			_ = SetRules([]Rule{
				{Field: FieldSubject, Pattern: fmt.Sprintf("BOUNCE-%d", i), Action: ActionHardBounce},
			})
		}(i)
	}
	wg.Wait()
}

func TestRegexPatternMatching(t *testing.T) {
	resetState()

	if err := SetRules([]Rule{
		{Field: FieldSubject, Pattern: `^BOUNCE-\d+$`, Action: ActionHardBounce},
	}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		subject string
		match   bool
	}{
		{"BOUNCE-123", true},
		{"BOUNCE-0", true},
		{"BOUNCE-abc", false},
		{"prefix BOUNCE-123", false},
		{"BOUNCE-123 suffix", false},
	}

	for _, tt := range tests {
		data := buildMessage("sender@example.com", "rcpt@example.com", tt.subject, "Body", nil)
		_, matched := Evaluate("sender@example.com", []string{"rcpt@example.com"}, data)
		if matched != tt.match {
			t.Errorf("subject %q: expected match=%v, got %v", tt.subject, tt.match, matched)
		}
	}
}

func TestMatchToFromEnvelope(t *testing.T) {
	resetState()

	if err := SetRules([]Rule{
		{Field: FieldTo, Pattern: "bcc-bounce@", Action: ActionHardBounce},
	}); err != nil {
		t.Fatal(err)
	}

	// The Bcc address is in the envelope (to) but not in the To header
	data := buildMessage("sender@example.com", "normal@example.com", "Hello", "Body", nil)

	_, matched := Evaluate("sender@example.com", []string{"normal@example.com", "bcc-bounce@example.com"}, data)
	if !matched {
		t.Fatal("expected rule to match on envelope to address")
	}
}
