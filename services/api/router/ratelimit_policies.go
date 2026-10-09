package router

import (
	"github.com/aspirit/xsocial/pkg/ratelimit"
)

// rateLimitPolicyDefs binds policy names to routes. Config only supplies
// name/requests/window; method, path and keying stay in code next to the routes
// so a renamed path can never silently drop its limit. First match wins.
var rateLimitPolicyDefs = []ratelimit.PolicyDef{
	{Name: ActionLogin, Method: ratelimit.MethodPost, Pattern: APIAuthLogin, MatchType: ratelimit.MatchTypeExact, KeyMode: ratelimit.KeyModeIPDeviceFallbackIP, DefaultWindow: ratelimit.Window1m},
	{Name: ActionSignup, Method: ratelimit.MethodPost, Pattern: APIAuthSignup, MatchType: ratelimit.MatchTypeExact, KeyMode: ratelimit.KeyModeIPDeviceFallbackIP, DefaultWindow: ratelimit.Window10m},
	{Name: ActionRefreshToken, Method: ratelimit.MethodPost, Pattern: APIAuthRefreshToken, MatchType: ratelimit.MatchTypeExact, KeyMode: ratelimit.KeyModeIPDeviceFallbackIP, DefaultWindow: ratelimit.Window1m},
	{Name: ActionWrite, Method: ratelimit.MethodPost, Pattern: APIPosts, MatchType: ratelimit.MatchTypePrefix, KeyMode: ratelimit.KeyModeIPDeviceFallbackIP, DefaultWindow: ratelimit.Window1m},
}

// GetRateLimitPolicyDef returns the policy definition for the given name, or false if unknown.
func GetRateLimitPolicyDef(name string) (ratelimit.PolicyDef, bool) {
	for i := range rateLimitPolicyDefs {
		if rateLimitPolicyDefs[i].Name == name {
			return rateLimitPolicyDefs[i], true
		}
	}

	return ratelimit.PolicyDef{}, false
}
