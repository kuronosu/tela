// Package router moves an app between screens, with React Router's names
// and a memory history (ADR-298).
//
// A [Router] holds the history, a stack of locations with back and forward;
// [Routes] renders the element of the route that matches the current
// location; [Link] and [UseNavigate] change it; [UseParams] and
// [UseLocation] read it:
//
//	func App() tela.Node {
//	    return router.Router{Children: Div(Class("flex h-full flex-col"),
//	        Div(Class("flex gap-4 p-3"),
//	            router.Link{To: "/", Children: Text("Home")},
//	            router.Link{To: "/notes", Children: Text("Notes")}),
//	        router.Routes(
//	            router.Route{Path: "/", Element: tela.C(Home)},
//	            router.Route{Path: "/notes", Element: tela.C(NotesLayout), Children: []router.Route{
//	                {Index: true, Element: tela.C(NoteList)},
//	                {Path: ":id", Element: tela.C(Note)},
//	            }},
//	            router.Route{Path: "*", Element: tela.C(NotFound)},
//	        ))}
//	}
//
//	func NotesLayout() tela.Node { return Div(H2(Text("Notes")), router.Outlet()) }
//
//	func Note() tela.Node {
//	    id := router.UseParams()["id"]
//	    return Text("Note " + id)
//	}
//
// # Routes
//
// A [Route] has a Path of static segments ("notes"), parameters (":id") and
// a trailing splat ("*"). Routes nest: a child's path is relative to its
// parent's, and the parent renders the matching child where it puts
// [Outlet]. When several routes match, the best ranks first, whatever their
// order: static segments beat parameters, parameters beat a splat, longer
// paths beat shorter ones. A route with Path "*" is the fallback for what no
// other matches. Matching ignores case and trailing slashes, and parameters
// are percent-decoded.
//
// # Navigating
//
// [Link] navigates when clicked or activated with Enter, and
// [Navigator.To], [Navigator.Replace], [Navigator.Back] and
// [Navigator.Forward] do from code. Relative paths resolve like React
// Router's: from the route the link or the component is in, and ".." goes up
// one route.
//
// Inside a Router the mouse's back and forward buttons go back and forward,
// and so do Alt+Left and Alt+Right (Command+[ and Command+] with the Mac
// flavor of key bindings), as in Chromium. A handler that stops or prevents
// the event keeps it from the router, and so does a text field that uses
// the key; the keys that move a text field's caret, such as Control+Left
// (Option+Left on a Mac), are not the router's.
//
// # Screens come and go
//
// The element of a route renders in an [h.Presence], keyed by the route: the
// screen a navigation leaves keeps its place, inert, while its [h.Exit]
// classes run, and leaves when they end; the one it enters can use the
// starting: variant. An element with an [h.LayoutID] on both screens
// animates between them (a shared element). The same route with other
// parameters keeps its element, which updates in place.
//
// # Links from outside
//
// With Router.Scheme set to "notes", the app's links of that scheme that
// the OS opens ("notes://notes/3") are navigations to their path
// ("/notes/3"); see desktop.Register for what makes the OS send them.
//
// # State
//
// The history is hook state of the Router, so `tela dev` keeps it across
// reloads (ADR-113). It is not saved when the app exits: to restore the
// last screen, store the location (UseLocation) in a setting and give it to
// Router.Initial.
package router
