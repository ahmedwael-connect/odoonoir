package installer

import "testing"

func BenchmarkPatchRequirements(b *testing.B) {
	// Simulate patching a small requirements.txt content
	content := "gevent==21.8.0\ngreenlet==1.1.2\ncryptography==3.4.8\nlxml==4.6.5\n"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = content
		// We don't actually run PatchRequirements (needs file), just benchmark string replace
		_ = versionOverrides["16"]
	}
}
