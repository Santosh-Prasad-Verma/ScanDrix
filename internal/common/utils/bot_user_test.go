package utils_test

import (
	"testing"

	"github.com/scandrix/backend/internal/common/utils"
)

func TestIsBotUser(t *testing.T) {
	botLogins := []string{
		"dependabot[bot]",
		"dependabot-preview[bot]",
		"renovate[bot]",
		"renovatebot",
		"github-actions[bot]",
		"gitlab-bot",
		"scandrix-bot",
		"drixy-bot",
		"mergify[bot]",
		// case-insensitive match
		"Dependabot[BOT]",
		"RENOVATE[bot]",
	}

	for _, login := range botLogins {
		if !utils.IsBotUser(login) {
			t.Errorf("expected %q to be treated as a bot", login)
		}
	}

	humanLogins := []string{
		"alex",
		"jdoe",
		"user-with-dashes",
		"Marie_Curie",
		// contains "robot" but not in our fragments list
		"robotron",
	}

	for _, login := range humanLogins {
		if utils.IsBotUser(login) {
			t.Errorf("expected %q to be treated as a human", login)
		}
	}

	falsyLogins := []string{
		"",
	}

	for _, login := range falsyLogins {
		if utils.IsBotUser(login) {
			t.Errorf("expected empty string to not be treated as a bot")
		}
	}
}
