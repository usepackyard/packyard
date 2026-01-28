package composer

import (
	"fmt"
	"regexp"
)

// Name/version validation patterns follow Composer's official rules
// (see Composer\Package\Loader\ValidatingArrayLoader). The character
// classes are intentionally strict — anything outside them is rejected
// to prevent path escape (e.g. "vendor/../../etc") and garbage input.

var (
	// packageNameRe matches "vendor/package" with the same rules Packagist enforces.
	packageNameRe = regexp.MustCompile(`^[a-z0-9]([_.-]?[a-z0-9]+)*/[a-z0-9](([_.]?|-{0,2})[a-z0-9]+)*$`)

	// versionRe is a permissive but safe version charset. It covers every
	// valid Composer version including dev branches and stability suffixes,
	// while rejecting path separators and shell metacharacters.
	versionRe = regexp.MustCompile(`^[a-zA-Z0-9._+-]{1,64}$`)

	// orgSlugRe — lowercase alphanum + hyphens, must start with a letter and
	// end with letter/digit. Length checked separately so the error is
	// specific.
	orgSlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

	// reservedOrgSlugs contains slugs that cannot be claimed because they
	// would shadow infrastructure subdomains, conflict with internal
	// concepts, or surprise users when the deployment expands. Add to this
	// list, never remove from it.
	reservedOrgSlugs = map[string]struct{}{
		"www": {}, "api": {}, "app": {}, "admin": {}, "docs": {},
		"billing": {}, "support": {}, "status": {}, "default": {},
		"help": {}, "mail": {}, "repo": {}, "cdn": {}, "static": {},
		"public": {}, "private": {}, "internal": {}, "system": {},
		"security": {}, "login": {}, "signup": {}, "register": {},
		"auth": {}, "dashboard": {}, "healthz": {}, "health": {},
		"root": {},
	}
)

// ValidatePackageName returns an error if name is not a valid Composer
// vendor/package identifier.
func ValidatePackageName(name string) error {
	if name == "" {
		return fmt.Errorf("package name is required")
	}
	if len(name) > 100 {
		return fmt.Errorf("package name too long")
	}
	if !packageNameRe.MatchString(name) {
		return fmt.Errorf("invalid package name %q: must match vendor/package format (lowercase alphanumerics, %q%q%q allowed)", name, "_", ".", "-")
	}
	return nil
}

// ValidateOrgSlug enforces the rules for organization slugs used as URL
// path segments and (eventually) subdomains. Lowercase alphanumerics +
// hyphens, 3–32 chars, must start with a letter and end with letter/digit.
// Reserved slugs are rejected to avoid collisions with infrastructure
// subdomains and product surfaces.
func ValidateOrgSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug is required")
	}
	if len(slug) < 3 {
		return fmt.Errorf("slug too short (min 3 characters)")
	}
	if len(slug) > 32 {
		return fmt.Errorf("slug too long (max 32 characters)")
	}
	if !orgSlugRe.MatchString(slug) {
		return fmt.Errorf("invalid slug %q: lowercase letters, digits and hyphens only; must start with a letter and not end with a hyphen", slug)
	}
	if _, reserved := reservedOrgSlugs[slug]; reserved {
		return fmt.Errorf("slug %q is reserved", slug)
	}
	return nil
}

// ValidateVersion returns an error if version is not a safe Composer
// version string.
func ValidateVersion(version string) error {
	if version == "" {
		return fmt.Errorf("version is required")
	}
	if !versionRe.MatchString(version) {
		return fmt.Errorf("invalid version %q: allowed chars are alphanumerics and . _ + -", version)
	}
	return nil
}
