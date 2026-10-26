package model

import (
	"strings"
	"testing"
)

func TestAnalyticsSettingsValidate(t *testing.T) {
	ok := []AnalyticsSettings{
		{},
		{Enabled: true, RetentionDays: 30, TopClients: AnalyticsClientsMemory},
		{InternalNetworks: []string{"198.51.100.0/24", "203.0.113.5", "2001:db8::/32", "fd00::1"}},
	}
	for i, a := range ok {
		if err := a.validate(); err != nil {
			t.Errorf("ok[%d]: %v", i, err)
		}
	}
	bad := map[string]AnalyticsSettings{
		"retention":    {RetentionDays: 366},
		"topClients":   {TopClients: "disk"},
		"not a cidr":   {InternalNetworks: []string{"10.0.0.0/8", "lan"}},
		"bad prefix":   {InternalNetworks: []string{"10.0.0.0/33"}},
		"too many":     {InternalNetworks: make([]string, MaxAnalyticsInternalNetworks+1)},
		"empty string": {InternalNetworks: []string{""}},
	}
	for name, a := range bad {
		err := a.validate()
		if err == nil || !strings.Contains(err.Error(), "analytics.") {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}
