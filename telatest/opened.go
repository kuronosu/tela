package telatest

import "slices"

// OpenedURLs returns the URLs the app opened in the system's application,
// oldest first, after settling the app: what tela.OpenURL was asked for (a
// link's destination, mail, a web page), and what a library that opens the
// browser does by it, such as package oauth's default sign-in, which sends
// the user to the authorization URL. The headless desktop records them
// instead of starting anything; paths opened with tela.OpenPath are not
// URLs and are left out. A URL the test has handled stays in the list.
//
//	app.Click(telatest.ByRole("button", "Sign in"))
//	urls := app.OpenedURLs()
//	// urls[0] is the authorization URL; play the browser:
//	// follow it with the fake server (oauthtest.Server.Browser).
func (a *App) OpenedURLs() []string {
	a.tb.Helper()
	a.settleAfter()
	var urls []string
	for _, r := range a.backend.Opened() {
		if !r.Path {
			urls = append(urls, r.Target)
		}
	}
	return slices.Clip(urls)
}
