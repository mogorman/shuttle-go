package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/hypebeast/go-osc/osc"
)

// bindingValue is a binding's raw JSON value: either a plain string
// (a single key/command) or an array of strings (a macro sequence).
type bindingValue struct {
	plain  string
	macros []string
}

var loadedConfiguration = &Config{}
var currentConfiguration *AppConfig

type Config struct {
	Apps []*AppConfig `json:"apps"`
}

type AppConfig struct {
	Name              string   `json:"name"`
	MatchWindowTitles []string `json:"match_window_titles"`
	MatchWMClass      []string `json:"match_wm_class"`
	SlowJog           *int     `json:"slow_jog"` // Time in millisecond to use slow jog
	Driver            string   `json:"driver"`
	windowTitleRegexps []*regexp.Regexp
	wmClassRegexps     []*regexp.Regexp
	Bindings          map[string]json.RawMessage `json:"bindings"`
	bindings          []*deviceBinding
}

func (ac *AppConfig) parse() error {
	if len(ac.MatchWindowTitles) == 0 && len(ac.MatchWMClass) == 0 {
		ac.windowTitleRegexps = []*regexp.Regexp{
			regexp.MustCompile(`.*`),
		}
		return nil
	}

	for _, window := range ac.MatchWindowTitles {
		re, err := regexp.Compile(window)
		if err != nil {
			return fmt.Errorf("Invalid regexp in window title match %q: %s", window, err)
		}
		ac.windowTitleRegexps = append(ac.windowTitleRegexps, re)
	}

	for _, class := range ac.MatchWMClass {
		re, err := regexp.Compile(class)
		if err != nil {
			return fmt.Errorf("Invalid regexp in wm_class match %q: %s", class, err)
		}
		ac.wmClassRegexps = append(ac.wmClassRegexps, re)
	}

	return nil
}

// matchesWindow reports whether the given window matches this app's configured
// matchers. Within each dimension, any regexp matching is enough (OR). When
// both match_window_titles and match_wm_class are present, both dimensions must
// match (AND), so e.g. you can target one specific Emacs window but not all of
// them.
func (ac *AppConfig) matchesWindow(title, wmClass string) bool {
	if len(ac.windowTitleRegexps) > 0 && len(ac.wmClassRegexps) > 0 {
		return ac.matchAny(ac.windowTitleRegexps, title) && ac.matchAny(ac.wmClassRegexps, wmClass)
	}
	if len(ac.windowTitleRegexps) > 0 {
		return ac.matchAny(ac.windowTitleRegexps, title)
	}
	if len(ac.wmClassRegexps) > 0 {
		return ac.matchAny(ac.wmClassRegexps, wmClass)
	}
	// No matchers configured: match everything.
	return true
}

func (ac *AppConfig) matchAny(regexps []*regexp.Regexp, value string) bool {
	for _, re := range regexps {
		if re.MatchString(value) {
			return true
		}
	}
	return false
}

type deviceBinding struct {
	rawKey   string
	rawValue string

	// Input
	heldButtons map[int]bool
	buttonDown  int
	otherKey    string

	driver    string
	oscClient *osc.Client

	// Output
	holdButtons []string
	pressButton string
	original    string
	description string
	macros      []string
}

func (ac *AppConfig) parseBindings() error {
	driverProtocol := "ydotool"
	var oscClient *osc.Client

	switch {
	case ac.Driver == "":
	case ac.Driver == "exec":
		driverProtocol = "exec"
	case ac.Driver == "ydotool":
	case strings.HasPrefix(ac.Driver, "osc://"):
		addr, err := url.Parse(ac.Driver)
		if err != nil {
			return fmt.Errorf("failed parsing osc:// address: %s", err)
		}
		hostParts := strings.Split(addr.Host, ":")
		if len(hostParts) != 2 {
			return fmt.Errorf("please specify a port for the osc:// address")
		}
		port, _ := strconv.ParseInt(hostParts[1], 10, 32)

		driverProtocol = "osc"
		oscClient = osc.NewClient(hostParts[0], int(port))
	default:
		return fmt.Errorf(`invalid driver %q, use one of: "ydotool" (default), "exec", "osc://address:port"`, ac.Driver)
	}

	for key, raw := range ac.Bindings {
		if strings.HasPrefix(key, "_") {
			continue
		}

		bv, err := decodeBindingValue(raw)
		if err != nil {
			return fmt.Errorf("binding %q: %s", key, err)
		}

		binding, description := bindingAndDescription(driverProtocol, bv.plain)
		newBinding := &deviceBinding{heldButtons: make(map[int]bool), rawKey: key, rawValue: bv.plain, original: binding, description: description, driver: driverProtocol, oscClient: oscClient, macros: bv.macros}

		// Input
		input := strings.Split(key, "+")
		for idx, part := range input {
			cleanPart := strings.TrimSpace(part)
			key := strings.ToUpper(cleanPart)
			if shuttleKeys[key] == 0 && !otherShuttleKeysUpper[key] {
				return fmt.Errorf("invalid shuttle device key map: %q doesn't exist", cleanPart)
			}
			if idx == len(input)-1 {
				if shuttleKeys[key] != 0 {
					newBinding.buttonDown = shuttleKeys[key]
				} else {
					newBinding.otherKey = key
				}
			} else {
				keyID := shuttleKeys[key]
				if keyID == 0 {
					return fmt.Errorf("binding %q, expects a button press, not a shuttle or jog movement", key)
				}
				newBinding.heldButtons[keyID] = true
			}
		}

		// Output
		// output := strings.Split(value, "+")
		// for idx, part := range output {
		// 	cleanPart := strings.TrimSpace(part)
		// 	buttonName := strings.ToUpper(cleanPart)
		// 	if keyboardKeysUpper[buttonName] == 0 {
		// 		return fmt.Errorf("keyboard key unknown: %q", cleanPart)
		// 	}
		// 	if idx == len(output)-1 {
		// 		newBinding.pressButton = buttonName
		// 	} else {
		// 		newBinding.holdButtons = append(newBinding.holdButtons, buttonName)
		// 	}
		// }

		ac.bindings = append(ac.bindings, newBinding)

		if *debugMode {
			fmt.Printf("BINDING: %#v\n", newBinding)
		}
	}

	return nil
}

var xdoDescriptionRE = regexp.MustCompile(`([^/]*)(\s*// *(.+))?`)
var oscDescriptionRE = regexp.MustCompile(`([^#]*)(\s*# *(.+))?`)

func bindingAndDescription(protocol, input string) (string, string) {
	re := xdoDescriptionRE
	if protocol == "osc" || protocol == "exec" {
		re = oscDescriptionRE
	}

	matches := re.FindStringSubmatch(input)
	if matches == nil {
		return input, ""
	}
	return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[3])
}

// decodeBindingValue interprets a binding's raw JSON value. It may be a plain
// string (a single key/command) or an array of strings (a macro sequence).
func decodeBindingValue(raw json.RawMessage) (bindingValue, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return bindingValue{}, fmt.Errorf("empty binding value")
	}

	// Array of strings -> macro sequence.
	if strings.HasPrefix(trimmed, "[") {
		var items []string
		if err := json.Unmarshal(raw, &items); err != nil {
			return bindingValue{}, fmt.Errorf("expected a string or an array of strings: %s", err)
		}
		return bindingValue{macros: items}, nil
	}

	// Plain string.
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return bindingValue{}, fmt.Errorf("expected a string or an array of strings: %s", err)
	}
	return bindingValue{plain: s}, nil
}

func LoadConfig(filename string) error {
	cnt, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	newConfig := &Config{}
	err = json.Unmarshal(cnt, &newConfig)
	if err != nil {
		return err
	}

	for _, app := range newConfig.Apps {
		if err := app.parse(); err != nil {
			return fmt.Errorf("Error parsing app %q's matchers: %s", app.Name, err)
		}

		if err := app.parseBindings(); err != nil {
			return fmt.Errorf("Error parsing app %q's bindings: %s", app.Name, err)
		}

	}

	loadedConfiguration = newConfig

	return nil
}
