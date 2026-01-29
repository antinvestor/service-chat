package deployment_test

import (
	"context"
	"os"
	"testing"

	"github.com/antinvestor/service-chat/tests/deployment"
)

// skipIfNoEnv skips the test if required environment variables are not set.
func skipIfNoEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("CHAT_SERVICE_ENDPOINT") == "" {
		t.Skip("Skipping deployment tests: CHAT_SERVICE_ENDPOINT not set")
	}
	if os.Getenv("CHAT_AUTH_TOKEN") == "" {
		t.Skip("Skipping deployment tests: CHAT_AUTH_TOKEN not set")
	}
}

// newTestClient creates a client for testing.
func newTestClient(t *testing.T) *deployment.Client {
	t.Helper()
	cfg, err := deployment.ConfigFromEnv()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}
	cfg.Verbose = testing.Verbose()

	client, err := deployment.NewClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Cleanup(func() {
		client.Cleanup(context.Background())
	})

	return client
}

// TestQuickCheck runs a quick health check against the deployed service.
func TestQuickCheck(t *testing.T) {
	skipIfNoEnv(t)

	cfg, err := deployment.ConfigFromEnv()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	client, err := deployment.NewClient(cfg)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	ctx := context.Background()
	defer client.Cleanup(ctx)

	if err := deployment.QuickCheck(ctx, client); err != nil {
		t.Fatalf("Quick check failed: %v", err)
	}
}

// TestRoomSuite runs all room operation tests.
func TestRoomSuite(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	suite := deployment.RoomTestSuite()
	runner := deployment.NewTestRunner(client)

	if err := runner.RunSuite(ctx, suite); err != nil {
		t.Fatalf("Suite execution failed: %v", err)
	}

	summary := runner.Summary()
	if !summary.AllPassed() {
		for _, result := range summary.Results {
			if !result.Passed {
				t.Errorf("Test %s failed: %v", result.Name, result.Error)
			}
		}
	}
}

// TestMessageSuite runs all message operation tests.
func TestMessageSuite(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	suite := deployment.MessageTestSuite()
	runner := deployment.NewTestRunner(client)

	if err := runner.RunSuite(ctx, suite); err != nil {
		t.Fatalf("Suite execution failed: %v", err)
	}

	summary := runner.Summary()
	if !summary.AllPassed() {
		for _, result := range summary.Results {
			if !result.Passed {
				t.Errorf("Test %s failed: %v", result.Name, result.Error)
			}
		}
	}
}

// TestSubscriptionSuite runs all subscription operation tests.
func TestSubscriptionSuite(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	suite := deployment.SubscriptionTestSuite()
	runner := deployment.NewTestRunner(client)

	if err := runner.RunSuite(ctx, suite); err != nil {
		t.Fatalf("Suite execution failed: %v", err)
	}

	summary := runner.Summary()
	if !summary.AllPassed() {
		for _, result := range summary.Results {
			if !result.Passed {
				t.Errorf("Test %s failed: %v", result.Name, result.Error)
			}
		}
	}
}

// TestLiveSuite runs all live/real-time operation tests.
func TestLiveSuite(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	suite := deployment.LiveTestSuite()
	runner := deployment.NewTestRunner(client)

	if err := runner.RunSuite(ctx, suite); err != nil {
		t.Fatalf("Suite execution failed: %v", err)
	}

	summary := runner.Summary()
	if !summary.AllPassed() {
		for _, result := range summary.Results {
			if !result.Passed {
				t.Errorf("Test %s failed: %v", result.Name, result.Error)
			}
		}
	}
}

// TestLifecycleSuite runs all end-to-end lifecycle tests.
func TestLifecycleSuite(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	suite := deployment.LifecycleTestSuite()
	runner := deployment.NewTestRunner(client)

	if err := runner.RunSuite(ctx, suite); err != nil {
		t.Fatalf("Suite execution failed: %v", err)
	}

	summary := runner.Summary()
	if !summary.AllPassed() {
		for _, result := range summary.Results {
			if !result.Passed {
				t.Errorf("Test %s failed: %v", result.Name, result.Error)
			}
		}
	}
}

// TestAllSuites runs all test suites.
func TestAllSuites(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	opts := &deployment.RunOptions{
		Verbose: testing.Verbose(),
		Cleanup: true,
	}

	summary, err := deployment.RunAll(ctx, client, opts)
	if err != nil {
		t.Fatalf("Test execution failed: %v", err)
	}

	if !summary.AllPassed() {
		t.Logf("Summary: %d/%d tests passed", summary.Passed, summary.Total)
		for _, result := range summary.Results {
			if !result.Passed {
				t.Errorf("Test %s failed: %v", result.Name, result.Error)
			}
		}
	}
}

// TestSmokeTests runs only smoke tests (quick validation).
func TestSmokeTests(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	opts := &deployment.RunOptions{
		Tags:    []string{"smoke"},
		Verbose: testing.Verbose(),
		Cleanup: true,
	}

	summary, err := deployment.RunAll(ctx, client, opts)
	if err != nil {
		t.Fatalf("Smoke test execution failed: %v", err)
	}

	if !summary.AllPassed() {
		for _, result := range summary.Results {
			if !result.Passed {
				t.Errorf("Smoke test %s failed: %v", result.Name, result.Error)
			}
		}
	}
}

// Individual test cases for fine-grained test selection

func TestCreateRoomBasic(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	test := &deployment.CreateRoomBasicTest{}
	if err := test.Run(ctx, client); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestSendTextMessage(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	test := &deployment.SendTextMessageTest{}
	if err := test.Run(ctx, client); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestGetHistoryBasic(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	test := &deployment.GetHistoryBasicTest{}
	if err := test.Run(ctx, client); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestAddMemberBasic(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	test := &deployment.AddMemberBasicTest{}
	if err := test.Run(ctx, client); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestCompleteRoomLifecycle(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	test := &deployment.CompleteRoomLifecycleTest{}
	if err := test.Run(ctx, client); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestActiveGroupScenario(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	test := &deployment.ActiveGroupScenarioTest{}
	if err := test.Run(ctx, client); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}

func TestMessageInteractionLifecycle(t *testing.T) {
	skipIfNoEnv(t)
	client := newTestClient(t)
	ctx := context.Background()

	test := &deployment.MessageInteractionLifecycleTest{}
	if err := test.Run(ctx, client); err != nil {
		t.Fatalf("Test failed: %v", err)
	}
}
