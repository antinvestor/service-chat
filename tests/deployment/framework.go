package deployment

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// TestResult represents the outcome of a single test.
type TestResult struct {
	Name     string
	Passed   bool
	Duration time.Duration
	Error    error
	Details  string
}

// TestCase defines the interface for a single test case.
type TestCase interface {
	// Name returns the test case name.
	Name() string

	// Description returns a human-readable description.
	Description() string

	// Run executes the test and returns nil on success.
	Run(ctx context.Context, client *Client) error

	// Tags returns categories for filtering (e.g., "room", "message", "subscription").
	Tags() []string
}

// TestSuite is a collection of related test cases.
type TestSuite struct {
	Name        string
	Description string
	Tests       []TestCase
	Setup       func(ctx context.Context, client *Client) error
	Teardown    func(ctx context.Context, client *Client) error
}

// TestRunner executes test suites and collects results.
type TestRunner struct {
	client    *Client
	results   []*TestResult
	verbose   bool
	tagFilter []string
}

// NewTestRunner creates a new test runner.
func NewTestRunner(client *Client) *TestRunner {
	return &TestRunner{
		client:  client,
		results: make([]*TestResult, 0),
		verbose: client.config.Verbose,
	}
}

// WithTagFilter filters tests by tags.
func (r *TestRunner) WithTagFilter(tags ...string) *TestRunner {
	r.tagFilter = tags
	return r
}

// RunSuite executes all tests in a suite.
func (r *TestRunner) RunSuite(ctx context.Context, suite *TestSuite) error {
	r.log("=== Running Suite: %s ===", suite.Name)
	r.log("Description: %s", suite.Description)
	r.log("")

	// Run suite setup
	if suite.Setup != nil {
		if err := suite.Setup(ctx, r.client); err != nil {
			return fmt.Errorf("suite setup failed: %w", err)
		}
	}

	// Run each test
	for _, tc := range suite.Tests {
		if !r.shouldRun(tc) {
			r.log("SKIP: %s (filtered by tags)", tc.Name())
			continue
		}

		result := r.runTest(ctx, tc)
		r.results = append(r.results, result)

		if result.Passed {
			r.log("PASS: %s (%v)", result.Name, result.Duration)
		} else {
			r.log("FAIL: %s (%v)", result.Name, result.Duration)
			r.log("  Error: %v", result.Error)
			if result.Details != "" {
				r.log("  Details: %s", result.Details)
			}
		}
	}

	// Run suite teardown
	if suite.Teardown != nil {
		if err := suite.Teardown(ctx, r.client); err != nil {
			r.log("WARNING: suite teardown failed: %v", err)
		}
	}

	r.log("")
	return nil
}

// runTest executes a single test case.
func (r *TestRunner) runTest(ctx context.Context, tc TestCase) *TestResult {
	start := time.Now()

	err := tc.Run(ctx, r.client)

	return &TestResult{
		Name:     tc.Name(),
		Passed:   err == nil,
		Duration: time.Since(start),
		Error:    err,
	}
}

// shouldRun checks if a test should run based on tag filters.
func (r *TestRunner) shouldRun(tc TestCase) bool {
	if len(r.tagFilter) == 0 {
		return true
	}

	tags := tc.Tags()
	for _, filter := range r.tagFilter {
		for _, tag := range tags {
			if tag == filter {
				return true
			}
		}
	}
	return false
}

// Results returns all test results.
func (r *TestRunner) Results() []*TestResult {
	return r.results
}

// Summary returns a summary of test results.
func (r *TestRunner) Summary() *TestSummary {
	summary := &TestSummary{
		Total:   len(r.results),
		Results: r.results,
	}

	for _, result := range r.results {
		if result.Passed {
			summary.Passed++
		} else {
			summary.Failed++
			summary.FailedTests = append(summary.FailedTests, result.Name)
		}
		summary.TotalDuration += result.Duration
	}

	return summary
}

// log prints a message if verbose mode is enabled.
func (r *TestRunner) log(format string, args ...any) {
	if r.verbose {
		fmt.Printf(format+"\n", args...)
	}
}

// TestSummary contains aggregated test results.
type TestSummary struct {
	Total         int
	Passed        int
	Failed        int
	TotalDuration time.Duration
	FailedTests   []string
	Results       []*TestResult
}

// String returns a formatted summary string.
func (s *TestSummary) String() string {
	var b strings.Builder

	b.WriteString("=== Test Summary ===\n")
	fmt.Fprintf(&b, "Total:    %d\n", s.Total)
	fmt.Fprintf(&b, "Passed:   %d\n", s.Passed)
	fmt.Fprintf(&b, "Failed:   %d\n", s.Failed)
	fmt.Fprintf(&b, "Duration: %v\n", s.TotalDuration)

	if s.Failed > 0 {
		b.WriteString("\nFailed Tests:\n")
		for _, name := range s.FailedTests {
			fmt.Fprintf(&b, "  - %s\n", name)
		}
	}

	return b.String()
}

// AllPassed returns true if all tests passed.
func (s *TestSummary) AllPassed() bool {
	return s.Failed == 0
}

// BaseTestCase provides common functionality for test cases.
type BaseTestCase struct {
	TestName        string
	TestDescription string
	TestTags        []string
}

// Name returns the test name.
func (b *BaseTestCase) Name() string {
	return b.TestName
}

// Description returns the test description.
func (b *BaseTestCase) Description() string {
	return b.TestDescription
}

// Tags returns the test tags.
func (b *BaseTestCase) Tags() []string {
	return b.TestTags
}

// AssertionError represents a test assertion failure.
type AssertionError struct {
	Message  string
	Expected any
	Actual   any
}

func (e *AssertionError) Error() string {
	if e.Expected != nil || e.Actual != nil {
		return fmt.Sprintf("%s (expected: %v, actual: %v)", e.Message, e.Expected, e.Actual)
	}
	return e.Message
}

// Assert provides common assertion functions.
type Assert struct{}

// NoError asserts that err is nil.
func (Assert) NoError(err error, msg string) error {
	if err != nil {
		return &AssertionError{Message: fmt.Sprintf("%s: %v", msg, err)}
	}
	return nil
}

// NotNil asserts that v is not nil.
func (Assert) NotNil(v any, msg string) error {
	if v == nil {
		return &AssertionError{Message: msg + ": expected non-nil value"}
	}
	return nil
}

// NotEmpty asserts that s is not empty.
func (Assert) NotEmpty(s string, msg string) error {
	if s == "" {
		return &AssertionError{Message: msg + ": expected non-empty string"}
	}
	return nil
}

// Equal asserts that expected equals actual.
func (Assert) Equal(expected, actual any, msg string) error {
	if expected != actual {
		return &AssertionError{
			Message:  msg,
			Expected: expected,
			Actual:   actual,
		}
	}
	return nil
}

// True asserts that condition is true.
func (Assert) True(condition bool, msg string) error {
	if !condition {
		return &AssertionError{Message: msg + ": expected true"}
	}
	return nil
}

// False asserts that condition is false.
func (Assert) False(condition bool, msg string) error {
	if condition {
		return &AssertionError{Message: msg + ": expected false"}
	}
	return nil
}

// GreaterOrEqual asserts that a >= b.
func (Assert) GreaterOrEqual(a, b int, msg string) error {
	if a < b {
		return &AssertionError{
			Message:  msg,
			Expected: fmt.Sprintf(">= %d", b),
			Actual:   a,
		}
	}
	return nil
}

// LessOrEqual asserts that a <= b.
func (Assert) LessOrEqual(a, b int, msg string) error {
	if a > b {
		return &AssertionError{
			Message:  msg,
			Expected: fmt.Sprintf("<= %d", b),
			Actual:   a,
		}
	}
	return nil
}

// Contains asserts that substr is contained in s.
func (Assert) Contains(s, substr, msg string) error {
	if !strings.Contains(s, substr) {
		return &AssertionError{
			Message:  msg,
			Expected: fmt.Sprintf("contains '%s'", substr),
			Actual:   s,
		}
	}
	return nil
}

// ErrorContains asserts that err contains the expected message.
func (Assert) ErrorContains(err error, expected, msg string) error {
	if err == nil {
		return &AssertionError{
			Message:  msg + ": expected error",
			Expected: expected,
			Actual:   nil,
		}
	}
	if !strings.Contains(err.Error(), expected) {
		return &AssertionError{
			Message:  msg,
			Expected: fmt.Sprintf("error containing '%s'", expected),
			Actual:   err.Error(),
		}
	}
	return nil
}

// Error asserts that err is not nil.
func (Assert) Error(err error, msg string) error {
	if err == nil {
		return &AssertionError{Message: msg + ": expected an error"}
	}
	return nil
}

// Len asserts that collection has expected length.
func (Assert) Len(length, expected int, msg string) error {
	if length != expected {
		return &AssertionError{
			Message:  msg,
			Expected: expected,
			Actual:   length,
		}
	}
	return nil
}

// MinLen asserts that length is at least min.
func (Assert) MinLen(length, min int, msg string) error {
	if length < min {
		return &AssertionError{
			Message:  msg,
			Expected: fmt.Sprintf("at least %d", min),
			Actual:   length,
		}
	}
	return nil
}
