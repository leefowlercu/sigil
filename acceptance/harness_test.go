package acceptance

import (
	"testing"

	"github.com/cucumber/godog"
	steps "github.com/leefowlercu/sigil/acceptance/steps"
)

func TestFeatures(t *testing.T) {
	t.Helper()

	suite := godog.TestSuite{
		Name:                "sigil-acceptance",
		ScenarioInitializer: steps.InitializeScenario,
		Options: &godog.Options{
			Format:   "pretty",
			Paths:    []string{"features"},
			TestingT: t,
		},
	}

	if suite.Run() != 0 {
		t.Fail()
	}
}
