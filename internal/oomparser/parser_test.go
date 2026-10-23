package oomparser

import (
	"fmt"
	"testing"
	"time"
)

func TestModernReports(t *testing.T) {
	uid := "12345678-1234-1234-1234-123456789abc"
	id := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	paths := []string{
		"/kubepods/burstable/pod" + uid + "/" + id,
		"/kubepods/besteffort/pod" + uid + "/" + id,
		"/kubepods/pod" + uid + "/" + id,
		"/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod12345678_1234_1234_1234_123456789abc.slice/cri-containerd-" + id + ".scope",
		"/kubepods.slice/kubepods-besteffort.slice/kubepods-besteffort-pod12345678_1234_1234_1234_123456789abc.slice/cri-containerd-" + id + ".scope",
		"/kubepods.slice/kubepods-pod12345678_1234_1234_1234_123456789abc.slice/cri-containerd-" + id + ".scope",
		"/system.slice/example.service",
	}
	for _, cgroup := range paths {
		for _, global := range []bool{false, true} {
			t.Run(fmt.Sprintf("global=%v%s", global, cgroup), func(t *testing.T) {
				constraint, limit, limitingCgroup := "CONSTRAINT_MEMCG", "oom_memcg=/limit", "/limit"
				if global {
					constraint, limit, limitingCgroup = "CONSTRAINT_NONE", "global_oom", "/"
				}
				line := fmt.Sprintf("oom-kill:constraint=%s,nodemask=(null),cpuset=/,mems_allowed=0,%s,task_memcg=%s,task=worker, child,pid=12345,uid=1000", constraint, limit, cgroup)
				parser := &Parser{}
				when := time.Unix(1700000000, 123)
				oom := parser.Parse(line, when)
				if oom == nil || oom.Pid != 12345 || oom.ProcessName != "worker, child" || oom.ContainerName != cgroup || oom.VictimContainerName != limitingCgroup || oom.Constraint != constraint || !oom.TimeOfDeath.Equal(when) {
					t.Fatalf("unexpected OOM: %+v", oom)
				}
				if duplicate := parser.Parse("Memory cgroup out of memory: Killed process 12345 (worker, child)", when); duplicate != nil {
					t.Fatalf("duplicate OOM: %+v", duplicate)
				}
			})
		}
	}
}

func TestLegacyReport(t *testing.T) {
	parser := &Parser{}
	when := time.Unix(1700000000, 0)
	for _, line := range []string{
		"worker invoked oom-killer: gfp_mask=0x0",
		"unrelated kernel log",
		"Task in /kubepods/pod123/container456 killed as a result of limit of /kubepods/pod123",
	} {
		if oom := parser.Parse(line, when); oom != nil {
			t.Fatalf("premature OOM: %+v", oom)
		}
	}
	oom := parser.Parse("Memory cgroup out of memory: Killed process 12345 (worker)", when)
	if oom == nil || oom.Pid != 12345 || oom.ProcessName != "worker" || oom.ContainerName != "/kubepods/pod123/container456" || oom.VictimContainerName != "/kubepods/pod123" || !oom.TimeOfDeath.Equal(when) {
		t.Fatalf("unexpected legacy OOM: %+v", oom)
	}
}

func TestNewReportDiscardsIncompleteLegacyContext(t *testing.T) {
	parser := &Parser{}
	when := time.Unix(1700000000, 0)
	parser.Parse("old invoked oom-killer:", when)
	parser.Parse("Task in /old-container killed as a result of limit of /old-limit", when)
	parser.Parse("new invoked oom-killer:", when)
	oom := parser.Parse("Out of memory: Killed process 12345 (new)", when)
	if oom == nil || oom.ContainerName != "/" || oom.VictimContainerName != "/" || oom.ProcessName != "new" {
		t.Fatalf("stale context leaked into new OOM: %+v", oom)
	}
}

func TestMalformedAndUnrelatedMessages(t *testing.T) {
	parser := &Parser{}
	for _, line := range []string{
		"", "unrelated kernel log", "Killed process 12345 (without header)",
		"oom-kill:constraint=CONSTRAINT_NONE,global_oom,task_memcg=/cgroup,task=worker,pid=invalid,uid=0",
		"oom-kill:constraint=CONSTRAINT_NONE,nodemask=(null),cpuset=/,mems_allowed=0,global_oom,task_memcg=/cgroup,task=worker,pid=999999999999999999999999999,uid=0",
	} {
		if oom := parser.Parse(line, time.Now()); oom != nil {
			t.Fatalf("unexpected OOM for %q: %+v", line, oom)
		}
	}
}
