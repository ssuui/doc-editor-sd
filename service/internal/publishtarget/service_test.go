package publishtarget

import (
	"testing"

	"doc-publish-server/internal/configloader"
)

func TestValidateLocalDirDefaults(t *testing.T) {
	cfg := &configloader.PublishTargetConfig{
		ID:        "local-main",
		Name:      "Local Main",
		Type:      "local_dir",
		TargetDir: "/tmp/site",
	}

	validated, err := validate(cfg)
	if err != nil {
		t.Fatalf("expected config to validate, got error: %v", err)
	}
	if validated.ModeDefault != "incremental" {
		t.Fatalf("expected default mode incremental, got %q", validated.ModeDefault)
	}
	if validated.BakDir != "/tmp/site/bak" {
		t.Fatalf("expected backup dir default, got %q", validated.BakDir)
	}
}

func TestValidateRejectsUnsafeID(t *testing.T) {
	cfg := &configloader.PublishTargetConfig{
		ID:              "../escape",
		Name:            "Unsafe",
		Type:            "s3",
		Bucket:          "bucket",
		Region:          "region",
		Endpoint:        "https://example.com",
		AccessKeyID:     "ak",
		SecretAccessKey: "sk",
	}

	if _, err := validate(cfg); err == nil {
		t.Fatal("expected unsafe id to be rejected")
	}
}
