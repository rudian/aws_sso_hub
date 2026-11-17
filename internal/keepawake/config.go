package keepawake

import (
	"time"

	"github.com/rudian/codepass/internal/api"
)

// DefaultConfig is what a fresh install gets: on, the screen too (as the owner asked), 15 minutes of grace for a
// task that waits for an answer, 8 hours of hard ceiling.
func DefaultConfig() api.KeepAwakeConfig {
	return api.KeepAwakeConfig{Enabled: true, ScreenToo: true, GraceMinutes: 15, CeilingHours: 8}
}

// ValidateConfig checks the values the Settings card offers.
func ValidateConfig(c api.KeepAwakeConfig) error {
	switch c.GraceMinutes {
	case 0, 5, 15, 60:
	default:
		return api.NewError(api.ErrInvalidInput, "grace time must be 5, 15 or 60 minutes, or 0 for no limit").WithParam("field", "graceMinutes")
	}
	switch c.CeilingHours {
	case 4, 8, 12:
	default:
		return api.NewError(api.ErrInvalidInput, "longest hold must be 4, 8 or 12 hours").WithParam("field", "ceilingHours")
	}
	return nil
}

func (m *Manager) graceDuration() (d time.Duration, forever bool) {
	if m.cfg.GraceMinutes <= 0 {
		return 0, true
	}
	return time.Duration(m.cfg.GraceMinutes) * time.Minute, false
}

func (m *Manager) ceilingDuration() time.Duration {
	h := m.cfg.CeilingHours
	if h <= 0 {
		h = DefaultConfig().CeilingHours
	}
	return time.Duration(h) * time.Hour
}

// Normalize gives a value the card does not offer its default and leaves the rest as it is: a stored configuration
// is never refused, because keep-awake must not stop the app from starting.
func Normalize(c api.KeepAwakeConfig) api.KeepAwakeConfig {
	def := DefaultConfig()
	switch c.GraceMinutes {
	case 0, 5, 15, 60:
	default:
		c.GraceMinutes = def.GraceMinutes
	}
	switch c.CeilingHours {
	case 4, 8, 12:
	default:
		c.CeilingHours = def.CeilingHours
	}
	return c
}

// Stored is the form A2-W keeps in settings.json (plan 3.1: every field has a safe zero value, so a settings file
// written before this feature, or by an older build, reads as the defaults):
//
//	"keepAwake": {"off": false, "screenOff": false, "graceMinutes": 0, "ceilingHours": 0}
//
// graceMinutes: 0 = 15, -1 = no limit ("一直"), 5 | 15 | 60 as is. ceilingHours: 0 = 8, 4 | 8 | 12 as is.
type Stored struct {
	Off          bool `json:"off"`
	ScreenOff    bool `json:"screenOff"`
	GraceMinutes int  `json:"graceMinutes"`
	CeilingHours int  `json:"ceilingHours"`
}

// FromStored maps the stored form to the configuration.
func FromStored(s Stored) api.KeepAwakeConfig {
	c := api.KeepAwakeConfig{Enabled: !s.Off, ScreenToo: !s.ScreenOff, GraceMinutes: s.GraceMinutes, CeilingHours: s.CeilingHours}
	switch s.GraceMinutes {
	case 0:
		c.GraceMinutes = 15
	case -1:
		c.GraceMinutes = 0
	}
	if s.CeilingHours == 0 {
		c.CeilingHours = 8
	}
	return Normalize(c)
}

// ToStored is the inverse of FromStored.
func ToStored(c api.KeepAwakeConfig) Stored {
	s := Stored{Off: !c.Enabled, ScreenOff: !c.ScreenToo, GraceMinutes: c.GraceMinutes, CeilingHours: c.CeilingHours}
	switch c.GraceMinutes {
	case 15:
		s.GraceMinutes = 0
	case 0:
		s.GraceMinutes = -1
	}
	if c.CeilingHours == 8 {
		s.CeilingHours = 0
	}
	return s
}
