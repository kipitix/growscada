package uiutil

// AuthorKey is the LocalStorage key of the author name the Save dialogs
// prefill. The name is a declaration, not an account (task 72_2; tasks 49/50
// bring Users), and both the Project and the Library mode share the one name,
// so the key lives here rather than in either of them.
const AuthorKey = "author:name"
