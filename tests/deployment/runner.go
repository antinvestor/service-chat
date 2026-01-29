package deployment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// AllTestSuites returns all available test suites.
func AllTestSuites() []*TestSuite {
	return []*TestSuite{
		RoomTestSuite(),
		MessageTestSuite(),
		MessageTypesTestSuite(),
		SubscriptionTestSuite(),
		LiveTestSuite(),
		GatewayTestSuite(),
		LifecycleTestSuite(),
		ProposalTestSuite(),
	}
}

// GetSuiteByName returns a test suite by its name.
func GetSuiteByName(name string) *TestSuite {
	for _, suite := range AllTestSuites() {
		if strings.EqualFold(suite.Name, name) || strings.EqualFold(strings.ReplaceAll(suite.Name, " ", "-"), name) {
			return suite
		}
	}
	return nil
}

// ListSuites returns a list of available suite names.
func ListSuites() []string {
	suites := AllTestSuites()
	names := make([]string, len(suites))
	for i, s := range suites {
		names[i] = s.Name
	}
	return names
}

// RunOptions configures how tests are run.
type RunOptions struct {
	// Suites to run (empty means all)
	Suites []string

	// Tags to filter tests (empty means all)
	Tags []string

	// Verbose output
	Verbose bool

	// StopOnFailure stops execution on first failure
	StopOnFailure bool

	// Cleanup resources after tests
	Cleanup bool
}

// DefaultRunOptions returns sensible defaults.
func DefaultRunOptions() *RunOptions {
	return &RunOptions{
		Verbose: true,
		Cleanup: true,
	}
}

// RunAll executes all test suites.
func RunAll(ctx context.Context, client *Client, opts *RunOptions) (*TestSummary, error) {
	if opts == nil {
		opts = DefaultRunOptions()
	}

	runner := NewTestRunner(client)
	if len(opts.Tags) > 0 {
		runner.WithTagFilter(opts.Tags...)
	}

	suites := AllTestSuites()

	// Filter suites if specified
	if len(opts.Suites) > 0 {
		filtered := make([]*TestSuite, 0)
		for _, s := range suites {
			for _, name := range opts.Suites {
				if strings.EqualFold(s.Name, name) || strings.Contains(strings.ToLower(s.Name), strings.ToLower(name)) {
					filtered = append(filtered, s)
					break
				}
			}
		}
		suites = filtered
	}

	if opts.Verbose {
		fmt.Println("===========================================")
		fmt.Println("  Chat Service Deployment Integration Tests")
		fmt.Println("===========================================")
		fmt.Printf("Endpoint: %s\n", client.Config().ServiceEndpoint)
		fmt.Printf("Suites:   %d\n", len(suites))
		fmt.Println("===========================================")
		fmt.Println()
	}

	for _, suite := range suites {
		if err := runner.RunSuite(ctx, suite); err != nil {
			if opts.StopOnFailure {
				return runner.Summary(), err
			}
		}
	}

	summary := runner.Summary()

	if opts.Verbose {
		fmt.Println(summary.String())
	}

	// Cleanup if requested
	if opts.Cleanup {
		if opts.Verbose {
			fmt.Println("Cleaning up test resources...")
		}
		errs := client.Cleanup(ctx)
		if len(errs) > 0 && opts.Verbose {
			fmt.Printf("Cleanup had %d errors (some may be expected)\n", len(errs))
		}
	}

	return summary, nil
}

// QuickCheck runs a minimal smoke test to verify service is operational.
func QuickCheck(ctx context.Context, client *Client) error {
	fmt.Println("Running quick health check...")

	// Test 1: Create a room
	room, err := client.CreateTestRoom(ctx, "healthcheck", nil)
	if err != nil {
		return fmt.Errorf("health check failed: cannot create room: %w", err)
	}
	fmt.Println("  ✓ Room creation works")

	// Test 2: Send a message
	_, err = client.SendTextMessage(ctx, room.GetId(), "Health check message")
	if err != nil {
		return fmt.Errorf("health check failed: cannot send message: %w", err)
	}
	fmt.Println("  ✓ Message sending works")

	// Test 3: Get history
	history, err := client.GetHistory(ctx, room.GetId(), 10, "")
	if err != nil {
		return fmt.Errorf("health check failed: cannot get history: %w", err)
	}
	if len(history.GetEvents()) == 0 {
		return errors.New("health check failed: history is empty")
	}
	fmt.Println("  ✓ History retrieval works")

	// Cleanup
	_ = client.Cleanup(ctx)

	fmt.Println("\nHealth check passed! Service is operational.")
	return nil
}

// PrintUsage prints usage information.
func PrintUsage() {
	fmt.Println(`
Chat Service Deployment Integration Tests
==========================================

ENVIRONMENT VARIABLES:
  CHAT_SERVICE_ENDPOINT  (required) Base URL of the chat service
  CHAT_AUTH_TOKEN        (required) JWT bearer token for authentication
  CHAT_REQUEST_TIMEOUT   (optional) Request timeout (default: 30s)
  CHAT_TEST_TIMEOUT      (optional) Overall test timeout (default: 10m)
  CHAT_VERBOSE           (optional) Enable verbose output (default: false)
  CHAT_NO_CLEANUP        (optional) Skip cleanup of test resources (default: false)
  CHAT_TEST_PREFIX       (optional) Prefix for test resources (default: "integration-test-")

AVAILABLE TEST SUITES:`)

	for _, suite := range AllTestSuites() {
		fmt.Printf("  - %s: %s\n", suite.Name, suite.Description)
	}

	fmt.Println(`
USAGE:
  # Run all tests
  CHAT_SERVICE_ENDPOINT=https://chat.example.com CHAT_AUTH_TOKEN=xxx go test ./tests/deployment/...

  # Run as standalone
  go run ./tests/deployment/cmd --quick    # Quick health check
  go run ./tests/deployment/cmd --all      # All tests
  go run ./tests/deployment/cmd --suite "Room Operations"  # Specific suite

EXAMPLES:
  export CHAT_SERVICE_ENDPOINT="https://chat.example.com"
  export CHAT_AUTH_TOKEN="eyJhbG..."
  export CHAT_VERBOSE=true

  # Run all tests with verbose output
  go test -v ./tests/deployment/...`)
}

// Main is the entry point for standalone execution.
func Main() {
	args := os.Args[1:]

	// Parse flags
	var (
		showHelp   bool
		quickCheck bool
		runAll     bool
		suiteName  string
		tags       []string
		listSuites bool
	)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "-h", "--help":
			showHelp = true
		case "-q", "--quick":
			quickCheck = true
		case "-a", "--all":
			runAll = true
		case "-l", "--list":
			listSuites = true
		case "-s", "--suite":
			if i+1 < len(args) {
				i++
				suiteName = args[i]
			}
		case "-t", "--tag":
			if i+1 < len(args) {
				i++
				tags = append(tags, args[i])
			}
		}
	}

	if showHelp {
		PrintUsage()
		os.Exit(0)
	}

	if listSuites {
		fmt.Println("Available test suites:")
		for _, name := range ListSuites() {
			fmt.Printf("  - %s\n", name)
		}
		os.Exit(0)
	}

	// Load configuration
	cfg, err := ConfigFromEnv()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		fmt.Fprintln(os.Stderr, "\nRun with --help for usage information")
		os.Exit(1)
	}

	// Create client
	client, err := NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Client initialization failed: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// Execute based on flags
	if quickCheck {
		if err := QuickCheck(ctx, client); err != nil {
			fmt.Fprintf(os.Stderr, "Quick check failed: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	opts := &RunOptions{
		Verbose: cfg.Verbose,
		Cleanup: cfg.CleanupAfterTests,
		Tags:    tags,
	}

	if suiteName != "" {
		opts.Suites = []string{suiteName}
	}

	if runAll || suiteName != "" || len(tags) > 0 {
		summary, err := RunAll(ctx, client, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Test execution error: %v\n", err)
			os.Exit(1)
		}

		if !summary.AllPassed() {
			os.Exit(1)
		}
		os.Exit(0)
	}

	// Default: show help
	PrintUsage()
}
