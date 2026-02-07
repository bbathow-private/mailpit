// Package bouncerules provides content-based bounce rules for the SMTPD server.
// Unlike chaos (random/probabilistic failures), bounce rules are deterministic
// and fire based on message content (subject, body, headers, recipients).
package bouncerules

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"sort"
	"sync"

	"github.com/goccy/go-yaml"
	"github.com/jhillyerd/enmime/v2"
	"github.com/lithammer/shortuuid/v4"
)

var (
	// Enabled is a flag to enable or disable bounce rules support
	Enabled = false

	mu    sync.RWMutex
	rules []Rule
)

// MatchField defines what part of the message to match against
type MatchField string

const (
	FieldTo      MatchField = "to"
	FieldFrom    MatchField = "from"
	FieldSubject MatchField = "subject"
	FieldBody    MatchField = "body"
	FieldHeader  MatchField = "header"
)

// Action defines the type of bounce response
type Action string

const (
	ActionHardBounce Action = "hard_bounce"
	ActionSoftBounce Action = "soft_bounce"
	ActionReject     Action = "reject"
	ActionQuotaFull  Action = "quota_full"
)

// Rule defines a content-based bounce rule
//
// swagger:model BounceRule
type Rule struct {
	// Unique identifier for the rule
	// example: abc123
	ID string `json:"id" yaml:"id"`

	// Message field to match against: to, from, subject, body, header
	// required: true
	// example: subject
	Field MatchField `json:"field" yaml:"field"`

	// Header name to match (only used when field is "header")
	// example: X-Test-Bounce
	HeaderName string `json:"header_name,omitempty" yaml:"header_name,omitempty"`

	// Regular expression pattern to match
	// required: true
	// example: BOUNCE-HARD
	Pattern string `json:"pattern" yaml:"pattern"`

	// compiled regex (not exported)
	compiled *regexp.Regexp

	// Bounce action: hard_bounce, soft_bounce, reject, quota_full
	// required: true
	// example: hard_bounce
	Action Action `json:"action" yaml:"action"`

	// Override the default SMTP error code for this action
	// example: 550
	ErrorCode int `json:"error_code,omitempty" yaml:"error_code,omitempty"`

	// Custom SMTP enhanced status message
	// example: 5.1.1 User unknown
	ErrorMessage string `json:"error_message,omitempty" yaml:"error_message,omitempty"`

	// Priority for rule evaluation order (lower = checked first)
	// example: 0
	Priority int `json:"priority" yaml:"priority"`
}

// SMTPResponse returns the SMTP error code and message for this rule
func (r Rule) SMTPResponse() (int, string) {
	code := r.ErrorCode
	msg := r.ErrorMessage

	if code == 0 || msg == "" {
		defaultCode, defaultMsg := defaultResponse(r.Action)
		if code == 0 {
			code = defaultCode
		}
		if msg == "" {
			msg = defaultMsg
		}
	}

	return code, msg
}

func defaultResponse(action Action) (int, string) {
	switch action {
	case ActionHardBounce:
		return 550, "5.1.1 User unknown"
	case ActionSoftBounce:
		return 451, "4.7.1 Try again later"
	case ActionReject:
		return 554, "5.7.1 Message rejected"
	case ActionQuotaFull:
		return 452, "4.2.2 Mailbox full"
	default:
		return 550, "5.1.1 User unknown"
	}
}

// Evaluate checks all rules against the message and returns the first matching rule.
// It parses the message using enmime and checks rules in priority order.
func Evaluate(from string, to []string, data []byte) (Rule, bool) {
	if !Enabled {
		return Rule{}, false
	}

	mu.RLock()
	currentRules := make([]Rule, len(rules))
	copy(currentRules, rules)
	mu.RUnlock()

	if len(currentRules) == 0 {
		return Rule{}, false
	}

	env, err := enmime.ReadEnvelope(bytes.NewReader(data))
	if err != nil {
		return Rule{}, false
	}

	for _, rule := range currentRules {
		if matchRule(rule, from, to, env) {
			return rule, true
		}
	}

	return Rule{}, false
}

func matchRule(rule Rule, from string, to []string, env *enmime.Envelope) bool {
	if rule.compiled == nil {
		return false
	}

	switch rule.Field {
	case FieldSubject:
		return rule.compiled.MatchString(env.GetHeader("Subject"))
	case FieldFrom:
		return rule.compiled.MatchString(from) || rule.compiled.MatchString(env.GetHeader("From"))
	case FieldTo:
		for _, addr := range to {
			if rule.compiled.MatchString(addr) {
				return true
			}
		}
		return rule.compiled.MatchString(env.GetHeader("To"))
	case FieldBody:
		if rule.compiled.MatchString(env.Text) {
			return true
		}
		return rule.compiled.MatchString(env.HTML)
	case FieldHeader:
		if rule.HeaderName == "" {
			return false
		}
		return rule.compiled.MatchString(env.GetHeader(rule.HeaderName))
	}

	return false
}

// SetRules validates and replaces all rules
func SetRules(newRules []Rule) error {
	for i := range newRules {
		if err := validateRule(&newRules[i]); err != nil {
			return err
		}
		if newRules[i].ID == "" {
			newRules[i].ID = shortuuid.New()
		}
	}

	sortRules(newRules)

	mu.Lock()
	rules = newRules
	mu.Unlock()

	return nil
}

// AddRule validates and adds a single rule
func AddRule(rule Rule) error {
	if err := validateRule(&rule); err != nil {
		return err
	}

	if rule.ID == "" {
		rule.ID = shortuuid.New()
	}

	mu.Lock()
	rules = append(rules, rule)
	sortRules(rules)
	mu.Unlock()

	return nil
}

// DeleteRule removes a rule by ID
func DeleteRule(id string) error {
	mu.Lock()
	defer mu.Unlock()

	for i, r := range rules {
		if r.ID == id {
			rules = append(rules[:i], rules[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("rule not found: %s", id)
}

// GetRules returns a copy of the current rules
func GetRules() []Rule {
	mu.RLock()
	defer mu.RUnlock()

	result := make([]Rule, len(rules))
	copy(result, rules)

	return result
}

// ClearRules removes all rules
func ClearRules() {
	mu.Lock()
	rules = []Rule{}
	mu.Unlock()
}

// yamlConfig is the structure for the YAML configuration file
type yamlConfig struct {
	Rules []Rule `yaml:"rules"`
}

// LoadFromYAML reads a YAML file and sets the rules
func LoadFromYAML(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("[bounce-rules] configuration not found or readable: %s", path)
	}

	var cfg yamlConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("[bounce-rules] error parsing YAML: %s", err.Error())
	}

	return SetRules(cfg.Rules)
}

func validateRule(rule *Rule) error {
	if rule.Pattern == "" {
		return fmt.Errorf("rule pattern cannot be empty")
	}

	compiled, err := regexp.Compile(rule.Pattern)
	if err != nil {
		return fmt.Errorf("invalid regex pattern %q: %s", rule.Pattern, err.Error())
	}
	rule.compiled = compiled

	switch rule.Field {
	case FieldTo, FieldFrom, FieldSubject, FieldBody, FieldHeader:
		// valid
	default:
		return fmt.Errorf("invalid field %q: must be one of to, from, subject, body, header", rule.Field)
	}

	if rule.Field == FieldHeader && rule.HeaderName == "" {
		return fmt.Errorf("header_name is required when field is \"header\"")
	}

	switch rule.Action {
	case ActionHardBounce, ActionSoftBounce, ActionReject, ActionQuotaFull:
		// valid
	default:
		return fmt.Errorf("invalid action %q: must be one of hard_bounce, soft_bounce, reject, quota_full", rule.Action)
	}

	if rule.ErrorCode != 0 && (rule.ErrorCode < 400 || rule.ErrorCode > 599) {
		return fmt.Errorf("error_code must be between 400 and 599")
	}

	return nil
}

func sortRules(r []Rule) {
	sort.SliceStable(r, func(i, j int) bool {
		return r[i].Priority < r[j].Priority
	})
}
