package sessionx

import (
	"net/http/httptest"
	"testing"
)

const androidUA = "Mozilla/5.0 (Linux; Android 14; SM-G973F Build/UP1A.231005.007) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36"

func TestDeviceFromUAKnownAndroidHandset(t *testing.T) {
	d, c := DeviceFromUA(androidUA)

	if d.Brand != "Samsung" {
		t.Fatalf("brand = %q, want Samsung", d.Brand)
	}
	if d.Name != "Galaxy S10" {
		t.Fatalf("name = %q, want Galaxy S10", d.Name)
	}
	if d.Code != "SM-G973F" {
		t.Fatalf("code = %q, want SM-G973F (the build suffix must be stripped)", d.Code)
	}
	if d.OS != "Android" || d.OSVersion != "14" {
		t.Fatalf("os = %q %q, want Android 14", d.OS, d.OSVersion)
	}
	if d.Browser != "Chrome" {
		t.Fatalf("browser = %q, want Chrome", d.Browser)
	}
	if d.Type != "Mobile" {
		t.Fatalf("type = %q, want Mobile", d.Type)
	}
	if !d.Known() {
		t.Fatal("a catalogued handset must report Known")
	}
	if !c.Human() {
		t.Fatal("a normal Chrome on Android is not a bot")
	}
}

func TestDeviceFromUAiPhone(t *testing.T) {
	const ua = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) " +
		"AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1"

	d, _ := DeviceFromUA(ua)

	if d.OS != "iOS" {
		t.Fatalf("os = %q, want iOS", d.OS)
	}
	if d.OSVersion != "17.4" {
		t.Fatalf("os version = %q, want 17.4", d.OSVersion)
	}
}

func TestDeviceFromUAUnknownInventsNothing(t *testing.T) {
	d, c := DeviceFromUA("some-agent-nobody-has-a-rule-for/1.0")

	if d.Raw != "some-agent-nobody-has-a-rule-for/1.0" {
		t.Fatalf("raw must be kept verbatim, got %q", d.Raw)
	}
	if d.Brand != "" || d.Name != "" || d.Type != "" {
		t.Fatalf("nothing may be guessed, got brand=%q name=%q type=%q", d.Brand, d.Name, d.Type)
	}
	if d.Known() {
		t.Fatal("an unrecognised agent is not Known")
	}
	_ = c
}

func TestClientFlagsABot(t *testing.T) {
	const ua = "Mozilla/5.0 (compatible; ClaudeBot/1.0; +claudebot@anthropic.com)"

	_, c := DeviceFromUA(ua)

	if c.Human() {
		t.Fatal("ClaudeBot must not be reported as human")
	}
	if c.BotName == "" {
		t.Fatalf("the bot must be named, got %+v", c)
	}
}

func TestClientFlagsACLITool(t *testing.T) {
	_, c := DeviceFromUA("curl/8.4.0")

	if c.Human() {
		t.Fatal("curl is a tool, not a human session")
	}
}

func TestDeviceFromUAEmpty(t *testing.T) {
	d, c := DeviceFromUA("")

	if d != (Device{}) {
		t.Fatalf("an empty user-agent yields the zero Device, got %+v", d)
	}
	if c != (Client{}) {
		t.Fatalf("an empty user-agent yields the zero Client, got %+v", c)
	}
}

func TestClientHumanRefusesToGuessOnEmptyUserAgent(t *testing.T) {
	_, c := DeviceFromUA("")

	if c.Human() {
		t.Fatalf("an absent user-agent must not be reported as human, got %+v", c)
	}
}

func TestDeviceFromRequestPrefersClientHints(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	// A reduced User-Agent — what Chrome increasingly sends — carrying the
	// real platform only in the hints.
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 10; K) "+
		"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Mobile Safari/537.36")
	req.Header.Set("Sec-CH-UA-Platform", `"Android"`)
	req.Header.Set("Sec-CH-UA-Platform-Version", `"14"`)
	req.Header.Set("Sec-CH-UA-Model", `"SM-G973F"`)

	d, _ := DeviceFromRequest(req)

	if d.OS != "Android" {
		t.Fatalf("os = %q, want Android from the hints", d.OS)
	}
	if d.Code != "SM-G973F" {
		t.Fatalf("code = %q, want the model from Sec-CH-UA-Model", d.Code)
	}
}
