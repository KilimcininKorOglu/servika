package provisioner

import (
	_ "embed"
	"os"
	"path/filepath"
)

// ── Brand assets (Lottie animations + player) ───────────────────────────────
// Kept EMBEDDED in the binary and written to a root-owned shared directory at
// Init; served by the `location ^~ /_gosp/` block in every vhost. This way:
//   - each domain's index.html stays SMALL (assets are shared + cached),
//   - a tenant CANNOT modify these files (they live outside its home),
//   - there is NO external dependency (CDN), so it works on offline/restricted
//     networks too.
//
// When the animation cannot load (an old vhost without /_gosp/, or JS disabled)
// the page falls back to an inline SVG drawing — never a broken image.

//go:embed brand/lottie.min.js
var lottieJS []byte

//go:embed brand/ready.json
var animReady []byte

//go:embed brand/notfound.json
var animNotFound []byte

// brandAssets maps file name → embedded content.
func brandAssets() map[string][]byte {
	return map[string][]byte{
		"lottie.min.js": lottieJS,
		"ready.json":    animReady,
		"notfound.json": animNotFound,
	}
}

// EnsureBrandAssets writes the animation assets into the shared directory
// (idempotent — untouched when the content is unchanged). Called from Init.
func EnsureBrandAssets() {
	if err := os.MkdirAll(errorPageDir, 0o755); err != nil {
		return
	}
	for name, content := range brandAssets() {
		path := filepath.Join(errorPageDir, name)
		if existing, err := os.ReadFile(path); err == nil && string(existing) == string(content) {
			continue
		}
		_ = os.WriteFile(path, content, 0o644)
	}
}
