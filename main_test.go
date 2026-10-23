//go:build linux
// +build linux

package main

import (
	"strings"
	"testing"
	"time"

	"github.com/euank/go-kmsg-parser/kmsgparser"
	"github.com/smpio/kube-oom-monitor/internal/oomparser"
)

func TestGlobalOOMStreamPreservesCgroupInNodeOOMMessage(t *testing.T) {
	when := time.Unix(1700000000, 123)
	cgroup := "/kubepods.slice/kubepods-burstable.slice/kubepods-burstable-pod123.slice/cri-containerd-456.scope"
	messages := make(chan kmsgparser.Message, 3)
	for _, line := range []string{
		"worker invoked oom-killer: gfp_mask=0x0",
		"oom-kill:constraint=CONSTRAINT_NONE,nodemask=(null),cpuset=/,mems_allowed=0,global_oom,task_memcg=" + cgroup + ",task=worker,pid=12345,uid=1000",
		"Out of memory: Killed process 12345 (worker)",
	} {
		// go-kmsg-parser preserves the real /dev/kmsg record's trailing newline.
		messages <- kmsgparser.Message{Message: line + "\n", Timestamp: when}
	}
	close(messages)
	events := make(chan *oomparser.Instance, 3)
	streamOoms(messages, events)
	var results []*oomparser.Instance
	for event := range events {
		results = append(results, event)
	}
	if len(results) != 1 {
		t.Fatalf("want one OOM event, got %d", len(results))
	}
	want := "pid:12345\nproc:worker\ntaskcg:" + cgroup + "\noomcg:/"
	if got := formatMessage(results[0]); got != want {
		t.Fatalf("message = %q, want %q", got, want)
	}
	if !results[0].TimeOfDeath.Equal(when) {
		t.Fatalf("wrong event timestamp: %v", results[0].TimeOfDeath)
	}
}

func TestFormatMessageRetainsLimitingCgroup(t *testing.T) {
	message := formatMessage(&oomparser.Instance{
		Pid: 42, ProcessName: "child", ContainerName: "/pod/container", VictimContainerName: "/pod",
	})
	if !strings.HasSuffix(message, "taskcg:/pod/container\noomcg:/pod") {
		t.Fatalf("cgroups missing from message: %q", message)
	}
}
