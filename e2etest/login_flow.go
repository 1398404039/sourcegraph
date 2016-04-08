package e2etest

import (
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"sourcegraph.com/sourcegraph/sourcegraph/go-sourcegraph/sourcegraph"

	"sourcegraph.com/sourcegraph/go-selenium"
)

func init() {
	Register(&Test{
		Name:        "login_flow",
		Description: "Logs in to an existing user account via the login page.",
		Func:        TestLoginFlow,
	})
}

func TestLoginFlow(t *T) error {
	// Create gRPC client connection so we can talk to the server. e2etest uses
	// the server's ID key for authentication, which means it can do ANYTHING with
	// no restrictions. Be careful!
	ctx, c := t.GRPCClient()

	// Create the test user account.
	testPassword := "e2etest"
	_, err := c.Accounts.Create(ctx, &sourcegraph.NewAccount{
		Login:    t.TestLogin,
		Email:    t.TestEmail,
		Password: testPassword,
	})
	if err != nil && grpc.Code(err) != codes.AlreadyExists {
		return err
	}

	// Get login page.
	t.Get(t.Endpoint("/login"))

	// Give the JS time to set element focus, etc.
	time.Sleep(1 * time.Second)

	// Validate username input field.
	username := t.FindElement(selenium.ById, "login")
	if username.TagName() != "input" {
		t.Fatalf("username TagName should be input, found", username.TagName())
	}
	if username.Text() != "" {
		t.Fatalf("username input field should be empty, found", username.Text())
	}
	if !username.IsDisplayed() {
		t.Fatalf("username input field should be displayed")
	}
	if !username.IsEnabled() {
		t.Fatalf("username input field should be enabled")
	}

	// Validate password input field.
	password := t.FindElement(selenium.ById, "password")
	if password.TagName() != "input" {
		t.Fatalf("password TagName should be input, found", password.TagName())
	}
	if password.Text() != "" {
		t.Fatalf("password input field should be empty, found", password.Text())
	}
	if !password.IsDisplayed() {
		t.Fatalf("password input field should be displayed")
	}
	if !password.IsEnabled() {
		t.Fatalf("password input field should be enabled")
	}
	if password.IsSelected() {
		t.Fatalf("password input field should not be selected")
	}

	// Enter username and password for test account.
	username.Click()
	username.SendKeys(t.TestLogin)
	password.Click()
	password.SendKeys(testPassword)

	// Click the submit button.
	submit := t.FindElement(selenium.ByCSSSelector, ".log-in > button.btn")
	submit.Click()

	// Wait for redirect.
	timeout := time.After(1 * time.Second)
	for {
		if t.CurrentURL() == t.Endpoint("/") {
			break
		}
		select {
		case <-timeout:
			t.Fatalf("expected redirect to homepage after sign-in; CurrentURL=%q\n", t.CurrentURL())
		default:
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
}
