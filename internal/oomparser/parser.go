// Package oomparser decodes kernel OOM messages without interpreting cgroup paths.
package oomparser

import (
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// Linux 5+ uses global_oom instead of oom_memcg for a node-wide OOM.
	summaryPattern = regexp.MustCompile(`^oom-kill:constraint=([^,]+),.*,(oom_memcg=(.*)|global_oom),task_memcg=(.*),task=(.*),pid=([0-9]+),uid=[0-9]+$`)
	legacyPattern  = regexp.MustCompile(`Task in (.*) killed as a result of limit of (.*)`)
	victimPattern  = regexp.MustCompile(`Killed process ([0-9]+) \((.*)\)`)
)

// Instance retains both the victim's cgroup and the cgroup whose limit was hit.
// For a global OOM there is no limiting cgroup; VictimContainerName is "/".
type Instance struct {
	Pid                 int
	ProcessName         string
	TimeOfDeath         time.Time
	ContainerName       string
	VictimContainerName string
	Constraint          string
}

// Parser assembles legacy multi-line reports and accepts modern summaries even
// if the preceding header was rate-limited or no longer in the ring buffer.
type Parser struct {
	pending *Instance
}

// Parse returns an instance once per report. The later "Killed process" line
// must not produce a second event after a modern summary has been emitted.
func (p *Parser) Parse(message string, timestamp time.Time) *Instance {
	// go-kmsg-parser retains record terminators; Go's $ anchor requires EOF.
	message = strings.TrimRight(message, "\r\n")
	if match := summaryPattern.FindStringSubmatch(message); match != nil {
		pid, err := strconv.Atoi(match[6])
		if err != nil {
			return nil
		}
		limitingCgroup := match[3]
		if limitingCgroup == "" {
			limitingCgroup = "/"
		}
		p.pending = nil
		return &Instance{
			Pid: pid, ProcessName: match[5], TimeOfDeath: timestamp,
			ContainerName: match[4], VictimContainerName: limitingCgroup,
			Constraint: match[1],
		}
	}
	if strings.Contains(message, "invoked oom-killer:") {
		p.pending = &Instance{ContainerName: "/", VictimContainerName: "/"}
		return nil
	}
	if p.pending == nil {
		return nil
	}
	if match := legacyPattern.FindStringSubmatch(message); match != nil {
		p.pending.ContainerName = path.Join("/", match[1])
		p.pending.VictimContainerName = path.Join("/", match[2])
		return nil
	}
	if match := victimPattern.FindStringSubmatch(message); match != nil {
		pid, err := strconv.Atoi(match[1])
		if err != nil {
			return nil
		}
		instance := p.pending
		p.pending = nil
		instance.Pid = pid
		instance.ProcessName = match[2]
		instance.TimeOfDeath = timestamp
		return instance
	}
	return nil
}
