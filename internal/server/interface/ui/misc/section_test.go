package misc

import (
	"strings"
	"testing"

	"github.com/maxence-charriere/go-app/v11/pkg/app"
)

// TestContents_ListsBothPages: the section's contents offer the Labels and the
// User name page (task 90).
func TestContents_ListsBothPages(t *testing.T) {
	s := New(nil)
	html := app.HTMLString(s.Render())
	for _, want := range []string{`data-misc-page="contents"`, `data-misc-entry="labels"`,
		`data-misc-entry="user"`, `data-page-title`, "Labels", "User name"} {
		if !strings.Contains(html, want) {
			t.Errorf("the contents lack %s: %s", want, html)
		}
	}
}

// TestBack_ReturnsToTheContents: a page's way back asks for the contents, and
// Root is told so the navigation line follows (task 90).
func TestBack_ReturnsToTheContents(t *testing.T) {
	s := New(nil)
	var reported []Page
	s.OnPage = func(page Page) { reported = append(reported, page) }

	if html := app.HTMLString(s.labels.Render()); !strings.Contains(html, "data-labels-page") {
		t.Errorf("the Labels page is not a page: %s", html)
	}
	if s.labels.OnBack == nil || s.username.OnBack == nil {
		t.Fatal("a page has no way back")
	}

	s.labels.OnBack()
	if s.page != PageContents {
		t.Errorf("the page after Back is %q, want the contents", s.page)
	}
	if len(reported) != 1 || reported[0] != PageContents {
		t.Errorf("the reported pages: %+v", reported)
	}
}

// TestShow_OpensThePageAndReportsIt: choosing a page shows it and tells Root,
// which keeps the navigation line and the stored page in step (task 90).
func TestShow_OpensThePageAndReportsIt(t *testing.T) {
	s := New(nil)
	var reported Page
	s.OnPage = func(page Page) { reported = page }

	s.Show(PageUserName)
	if s.page != PageUserName || reported != PageUserName {
		t.Errorf("the page is %q, reported %q", s.page, reported)
	}
	if html := app.HTMLString(s.Render()); !strings.Contains(html, `data-username-page`) {
		t.Errorf("the User name page is not shown: %s", html)
	}
}
