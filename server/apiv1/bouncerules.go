package apiv1

import (
	"encoding/json"
	"net/http"

	"github.com/axllent/mailpit/internal/smtpd/bouncerules"
)

// GetBounceRules returns the current bounce rules
func GetBounceRules(w http.ResponseWriter, _ *http.Request) {
	// swagger:route GET /api/v1/bounce-rules testing getBounceRules
	//
	// # Get bounce rules
	//
	// Returns the current bounce rules configuration.
	// This API route will return an error if bounce rules are not enabled at runtime.
	//
	//	Produces:
	//	  - application/json
	//
	//	Schemes: http, https
	//
	//	Responses:
	//	  200: BounceRulesResponse
	//	  400: ErrorResponse

	if !bouncerules.Enabled {
		httpError(w, "Bounce rules are not enabled")
		return
	}

	rules := bouncerules.GetRules()

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(rules); err != nil {
		httpError(w, err.Error())
	}
}

// SetBounceRules replaces all bounce rules
func SetBounceRules(w http.ResponseWriter, r *http.Request) {
	// swagger:route PUT /api/v1/bounce-rules testing setBounceRulesParams
	//
	// # Set bounce rules
	//
	// Replace all bounce rules with the provided set and return the updated values.
	// This API route will return an error if bounce rules are not enabled at runtime.
	//
	//	Consumes:
	//	  - application/json
	//
	//	Produces:
	//	  - application/json
	//
	//	Schemes: http, https
	//
	//	Responses:
	//	  200: BounceRulesResponse
	//	  400: ErrorResponse

	if !bouncerules.Enabled {
		httpError(w, "Bounce rules are not enabled")
		return
	}

	var data []bouncerules.Rule

	decoder := json.NewDecoder(r.Body)

	if err := decoder.Decode(&data); err != nil {
		httpError(w, err.Error())
		return
	}

	if err := bouncerules.SetRules(data); err != nil {
		httpError(w, err.Error())
		return
	}

	rules := bouncerules.GetRules()

	w.Header().Add("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(rules); err != nil {
		httpError(w, err.Error())
	}
}

// ClearBounceRules removes all bounce rules
func ClearBounceRules(w http.ResponseWriter, _ *http.Request) {
	// swagger:route DELETE /api/v1/bounce-rules testing clearBounceRules
	//
	// # Clear bounce rules
	//
	// Remove all bounce rules.
	// This API route will return an error if bounce rules are not enabled at runtime.
	//
	//	Produces:
	//	  - text/plain
	//
	//	Schemes: http, https
	//
	//	Responses:
	//	  200: OKResponse
	//	  400: ErrorResponse

	if !bouncerules.Enabled {
		httpError(w, "Bounce rules are not enabled")
		return
	}

	bouncerules.ClearRules()

	w.Header().Add("Content-Type", "text/plain")
	_, _ = w.Write([]byte("ok"))
}
