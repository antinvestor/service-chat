# Deployment Integration Tests

Comprehensive integration tests for validating a deployed Chat Service instance. These tests exercise all API endpoints against a real deployment to ensure the service is functioning correctly.

## Features

- **Full API Coverage**: Tests all 11 ChatService endpoints
- **Lifecycle Testing**: Complete room lifecycle from creation to deletion
- **Real-time Features**: Tests typing indicators, read markers, and receipts
- **Role-based Testing**: Validates permission hierarchies
- **Extensible Framework**: Easy to add new tests using the TestCase interface
- **Multiple Run Modes**: Go test integration, CLI tool, or programmatic usage

## Test Suites

| Suite | Description |
|-------|-------------|
| Room Operations | Room creation, update, search, and deletion |
| Message Operations | Sending messages, history, reactions, validation |
| Subscription Operations | Member management, role updates, permissions |
| Live Operations | Typing, read markers, receipts, presence |
| Lifecycle Tests | End-to-end scenarios covering full group lifecycles |

## Quick Start

### 1. Set Environment Variables

```bash
export CHAT_SERVICE_ENDPOINT="https://chat.example.com"
export CHAT_AUTH_TOKEN="eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
export CHAT_VERBOSE=true  # Optional: enable verbose output
```

### 2. Run Tests

**Using Go Test:**
```bash
# Run all tests
go test -v ./tests/deployment/...

# Run specific suite
go test -v -run TestRoomSuite ./tests/deployment/...

# Run smoke tests only
go test -v -run TestSmokeTests ./tests/deployment/...

# Run a specific test
go test -v -run TestCompleteRoomLifecycle ./tests/deployment/...
```

**Using CLI Tool:**
```bash
# Build the CLI
go build -o integration-test ./tests/deployment/cmd

# Quick health check
./integration-test --quick

# Run all suites
./integration-test --all

# Run specific suite
./integration-test --suite "Room Operations"

# List available suites
./integration-test --list
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `CHAT_SERVICE_ENDPOINT` | Yes | - | Base URL of the deployed service |
| `CHAT_AUTH_TOKEN` | Yes | - | JWT bearer token for authentication |
| `CHAT_REQUEST_TIMEOUT` | No | 30s | Timeout for individual API calls |
| `CHAT_TEST_TIMEOUT` | No | 10m | Overall test suite timeout |
| `CHAT_VERBOSE` | No | false | Enable verbose logging |
| `CHAT_NO_CLEANUP` | No | false | Skip cleanup of test resources |
| `CHAT_TEST_PREFIX` | No | "integration-test-" | Prefix for test resource names |

## Test Categories (Tags)

Tests are tagged for filtering:

- `smoke` - Quick validation tests
- `room` - Room-related tests
- `message` - Message-related tests
- `subscription` - Subscription-related tests
- `live` - Real-time feature tests
- `lifecycle` - End-to-end lifecycle tests
- `e2e` - Full end-to-end scenarios

## Writing New Tests

### 1. Implement the TestCase Interface

```go
type MyNewTest struct {
    deployment.BaseTestCase
}

func (t *MyNewTest) Name() string        { return "MyNewTest" }
func (t *MyNewTest) Description() string { return "Description of what it tests" }
func (t *MyNewTest) Tags() []string      { return []string{"custom", "smoke"} }

func (t *MyNewTest) Run(ctx context.Context, client *deployment.Client) error {
    var a deployment.Assert

    // Create test resources
    room, err := client.CreateTestRoom(ctx, "my-test-room", nil)
    if err := a.NoError(err, "CreateRoom should succeed"); err != nil {
        return err
    }

    // Perform test operations
    _, err = client.SendTextMessage(ctx, room.GetId(), "Hello")
    if err := a.NoError(err, "SendMessage should succeed"); err != nil {
        return err
    }

    // Verify results
    history, err := client.GetHistory(ctx, room.GetId(), 10, "")
    if err := a.NoError(err, "GetHistory should succeed"); err != nil {
        return err
    }

    if err := a.MinLen(len(history.GetEvents()), 1, "Should have events"); err != nil {
        return err
    }

    return nil
}
```

### 2. Add to a Test Suite

```go
func CustomTestSuite() *deployment.TestSuite {
    return &deployment.TestSuite{
        Name:        "Custom Tests",
        Description: "My custom test suite",
        Tests: []deployment.TestCase{
            &MyNewTest{},
            // Add more tests...
        },
    }
}
```

### 3. Register in AllTestSuites (optional)

Edit `runner.go` to include your suite in `AllTestSuites()`.

## Client Helper Methods

The `Client` provides convenience methods for common operations:

```go
// Room operations
client.CreateTestRoom(ctx, name, members)
client.CreateTestRoomWithMetadata(ctx, name, description, isPrivate, metadata)
client.UpdateRoom(ctx, roomID, name, topic)

// Message operations
client.SendTextMessage(ctx, roomID, text)
client.SendBatchMessages(ctx, roomID, messages)
client.GetHistory(ctx, roomID, limit, cursor)

// Subscription operations
client.AddMembers(ctx, roomID, members)
client.RemoveMembers(ctx, roomID, subscriptionIDs)
client.SearchSubscriptions(ctx, roomID)
client.UpdateSubscriptionRole(ctx, roomID, subscriptionID, roles)

// Live operations
client.SendTypingIndicator(ctx, roomID, subscriptionID, typing)
client.SendReadMarker(ctx, roomID, upToEventID, subscriptionID)
client.SendLiveUpdate(ctx, commands)
```

## Assertions

The `Assert` helper provides common assertions:

```go
var a deployment.Assert

a.NoError(err, "message")
a.NotNil(value, "message")
a.NotEmpty(str, "message")
a.Equal(expected, actual, "message")
a.True(condition, "message")
a.False(condition, "message")
a.MinLen(length, min, "message")
a.GreaterOrEqual(a, b, "message")
a.Contains(str, substr, "message")
a.Error(err, "message")
a.ErrorContains(err, expected, "message")
```

## CI/CD Integration

Add to your CI pipeline:

```yaml
test-deployment:
  stage: integration
  script:
    - export CHAT_SERVICE_ENDPOINT="${STAGING_CHAT_ENDPOINT}"
    - export CHAT_AUTH_TOKEN="${STAGING_AUTH_TOKEN}"
    - export CHAT_VERBOSE=true
    - go test -v -timeout 20m ./tests/deployment/...
  only:
    - main
    - staging
```

## Troubleshooting

### Tests skip with "CHAT_SERVICE_ENDPOINT not set"

Set the required environment variables before running tests.

### Authentication failures

Ensure your `CHAT_AUTH_TOKEN` is a valid JWT token with appropriate permissions.

### Cleanup issues

If tests fail mid-execution, test resources may be left behind. Run with `CHAT_NO_CLEANUP=true` to debug, then manually clean up or re-run tests with cleanup enabled.

### Timeout errors

Increase `CHAT_REQUEST_TIMEOUT` for slow networks or large operations.
