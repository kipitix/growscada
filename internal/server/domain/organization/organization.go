// Package organization will host the Organization aggregate: the tenant that
// owns Projects and Libraries (ADR 0006). Until Users exist (task 49) the
// server runs with one Organization seeded by the migrations, so for now it
// exists only as the identity type of that key; no aggregate behavior is
// defined yet.
package organization

// Organization - identity marker for the tenant owning Projects.
type Organization interface {
}
