package sessionx

import (
	"net/http"
	"strings"

	"github.com/bakhod1r/devicex"
	"github.com/bakhod1r/uax"
)

// Device is the hardware and software a session was opened from, as far as
// the request can be trusted to say.
//
// Fields are empty when nothing recognised them. This package never
// substitutes "Unknown" or "Other": an empty Brand means no rule and no
// catalogue entry answered, which a caller can act on, while a placeholder
// cannot be told apart from a real value.
type Device struct {
	// Raw is the User-Agent exactly as it arrived.
	Raw string

	// Code is the model code, from Sec-CH-UA-Model when the client sent it
	// and from the User-Agent otherwise, with any build suffix stripped.
	Code string

	// Brand is the manufacturer, normalised by devicex.
	Brand string

	// Name is the marketing name, for example "Galaxy S10".
	Name string

	// Family is the product line, when a rule identifies one.
	Family string

	// Type is the form factor: Mobile, Tablet, Desktop, TV, Console, Watch,
	// Car, XR, EInk, Embedded, or empty.
	Type string

	// OS is the operating system name.
	OS string

	// OSVersion is the operating system version.
	OSVersion string

	// Browser is the browser name, for example "Chrome".
	Browser string

	// BrowserVersion is the browser version.
	BrowserVersion string

	// Confidence is the device identification's own 0..1 score, 0 when
	// nothing matched.
	Confidence float64
}

// Known reports whether anything at all was identified.
func (d Device) Known() bool { return d.Brand != "" || d.OS != "" || d.Browser != "" }

// Client is what kind of thing opened the session, and how well its own
// claims hold together.
//
// It exists because "a session" and "a person" are not the same thing. A
// crawler, a CLI tool and a headless browser all open sessions, and an
// application that cannot tell them apart cannot rate-limit them apart
// either.
type Client struct {
	// Kind is uax's classification: Human, Bot, AICrawler, AIAgent, Tool,
	// Automation, or Unknown.
	Kind string

	// BotName is the crawler's name when one identified itself. It is a
	// claim, not a verified fact — a User-Agent saying "Googlebot" is not
	// proof that Google sent it.
	BotName string

	// AppName is the native application hosting an embedded WebView, for
	// example "Instagram".
	AppName string

	// AutomationName is the headless browser or automation framework that
	// declared itself. Its absence is never evidence of a human: a driver
	// configured to hide leaves no marker.
	AutomationName string

	// SpoofScore rises from 0 towards 1 as the client's claims stop fitting
	// together. It is recorded rather than acted on: what to do about a
	// suspicious session is the application's policy, not this library's.
	SpoofScore float64
}

// Human reports whether the client looks like a person's browser — not a
// bot, not a tool, not declared automation.
func (c Client) Human() bool {
	return c.Kind == string(uax.KindHuman) || (c.Kind == "" && c.BotName == "")
}

// parser is the shared uax parser, wired to devicex so that a model code
// becomes a handset name. The cache is on because a server sees the same
// handful of User-Agent strings over and over, and re-parsing each one per
// request is wasted work.
var parser = uax.New(uax.Config{
	Device: devicex.Describe,
	Cache:  uax.CacheConfig{Enabled: true, Size: 8192},
})

// DeviceFromUA collects what a bare User-Agent string discloses.
//
// Prefer DeviceFromRequest where a request is in hand: Client Hints are
// structured, are not subject to the User-Agent reduction that is steadily
// emptying the legacy string, and are what a modern Chrome actually carries
// the platform and model in.
func DeviceFromUA(ua string) (Device, Client) {
	if strings.TrimSpace(ua) == "" {
		return Device{}, Client{}
	}
	return flatten(parser.Parse(ua))
}

// DeviceFromRequest collects from the whole request: the User-Agent, the
// Client Hints, and the fetch metadata around them.
func DeviceFromRequest(r *http.Request) (Device, Client) {
	if r == nil || strings.TrimSpace(r.UserAgent()) == "" {
		return Device{}, Client{}
	}
	return flatten(parser.ParseRequest(r))
}

// flatten turns a uax.Client into the two flat structs a session row holds.
// Nothing is inferred here; every value is copied or left empty.
func flatten(c *uax.Client) (Device, Client) {
	if c == nil {
		return Device{}, Client{}
	}

	d := Device{
		Raw:        c.Raw,
		Code:       c.Device.Model,
		Brand:      c.Device.Brand,
		Name:       c.Device.Name,
		Family:     c.Device.Family,
		Type:       string(c.Device.Type),
		OS:         c.OS.Name,
		Browser:    c.Browser.Name,
		Confidence: float64(c.Device.Confidence),
	}
	if c.OS.Version.Known() {
		d.OSVersion = c.OS.Version.String()
	}
	if c.Browser.Version.Known() {
		d.BrowserVersion = c.Browser.Version.String()
	}

	cl := Client{
		Kind:       string(c.Kind),
		SpoofScore: float64(c.SpoofScore),
	}
	if c.Bot != nil {
		cl.BotName = c.Bot.Name
	}
	if c.App != nil {
		cl.AppName = c.App.Name
	}
	if c.Automation != nil {
		cl.AutomationName = c.Automation.Name
	}
	return d, cl
}
