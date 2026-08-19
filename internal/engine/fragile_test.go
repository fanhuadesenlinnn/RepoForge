package engine

import (
	"testing"

	"github.com/fanhuadesenlinnn/RepoForge/internal/repo"
	"github.com/fanhuadesenlinnn/RepoForge/internal/upstream"
)

func TestFragileTuneCapsKylin(t *testing.T) {
	d := newDownloader(8, repo.SegmentSmart, 20, true)
	d.applyFragileTune("https://update.cs2c.com.cn/NS/V10/V10SP3-2403/os/adv/lic/base/x86_64/")
	if d.jobs != upstream.FragileMaxJobs {
		t.Fatalf("jobs = %d, want %d", d.jobs, upstream.FragileMaxJobs)
	}
	if d.segment != repo.SegmentDisabled {
		t.Fatalf("segment = %d, want disabled", d.segment)
	}
}

func TestFragileTuneLeavesFriendlyMirrors(t *testing.T) {
	d := newDownloader(8, repo.SegmentSmart, 20, true)
	d.applyFragileTune("https://mirrors.aliyun.com/centos-vault/8.5.2111/BaseOS/x86_64/os/")
	if d.jobs != 8 {
		t.Fatalf("jobs = %d, want 8", d.jobs)
	}
	if d.segment != repo.SegmentSmart {
		t.Fatalf("segment = %d, want smart", d.segment)
	}
}

func TestFragileTuneAlreadyLowConcurrency(t *testing.T) {
	d := newDownloader(1, repo.SegmentDisabled, 20, true)
	d.applyFragileTune("https://update.cs2c.com.cn/x")
	if d.jobs != 1 {
		t.Fatalf("jobs = %d, want 1 (already below cap)", d.jobs)
	}
}
