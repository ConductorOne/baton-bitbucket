package config

import (
	"testing"

	"github.com/conductorone/baton-sdk/pkg/test"
)

func TestConfigs(t *testing.T) {
	testCases := []test.TestCase{
		{
			map[string]string{},
			false,
			"empty",
		},
		{
			map[string]string{
				"app-password":    "1",
				"consumer-key":    "1",
				"consumer-secret": "1",
				"token":           "1",
				"username":        "1",
				"workspaces":      "1",
			},
			false,
			"in conflict",
		},
		{
			map[string]string{
				"app-password": "1",
				"username":     "1",
			},
			true,
			"username + password",
		},
		{
			map[string]string{
				"token": "1",
			},
			true,
			"token",
		},
		{
			map[string]string{
				"consumer-key":    "1",
				"consumer-secret": "1",
			},
			true,
			"token",
		},
		{
			map[string]string{
				"token":      "1",
				"workspaces": "1",
			},
			true,
			"workspaces",
		},
	}

	test.ExerciseTestCases(t, ConfigurationSchema, nil, testCases)
}
