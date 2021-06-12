package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/euank/go-kmsg-parser/kmsgparser"
	"github.com/google/cadvisor/utils/oomparser"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

func main() {
	startedAt := time.Now()

	flagset := flag.CommandLine
	masterURL := flagset.String("master", "", "kubernetes api server url")
	kubeconfigPath := flagset.String("kubeconfig", "", "path to kubeconfig file")
	nodeName := flagset.String("nodeName", "", "name of the node to bind events")
	eventReason := flagset.String("eventReason", "NodeOOM", "event reason")

	klog.InitFlags(flagset)
	flag.Parse()

	if *nodeName == "" {
		klog.Fatalln("Please specify nodeName")
		return
	}

	config, err := clientcmd.BuildConfigFromFlags(*masterURL, *kubeconfigPath)
	if err != nil {
		klog.Fatalln(err)
		return
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalln(err)
		return
	}

	clockDrift, err := syncClock()
	if err != nil {
		klog.Fatalln(err)
		return
	}
	klog.Infof("Clock drift: %v", clockDrift)

	outStream := make(chan *oomparser.OomInstance)
	oomLog, err := oomparser.New()
	if err != nil {
		klog.Fatalf("Couldn't make a new oomparser. %v", err)
		return
	}

	go oomLog.StreamOoms(outStream)
	klog.Infoln("Reading the OOM stream")

	for oom := range outStream {
		klog.Infof("OOM: %+v", oom)
		t := metav1.Time{Time: oom.TimeOfDeath.Add(*clockDrift)}
		klog.Infof("Calibrated time: %v", t.Time)

		if t.Time.Before(startedAt) {
			klog.Infof("Skipping this old event")
			continue
		}

		now := time.Now()
		klog.Infof("Current time: %v", now)
		// https://github.com/smpio/kube-oom-monitor/issues/1
		t.Time = now

		event := &v1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%v.%x", *nodeName, oom.Pid),
				Namespace: metav1.NamespaceDefault,
			},
			InvolvedObject: v1.ObjectReference{
				Kind: "Node",
				Name: *nodeName,
			},
			Reason:         *eventReason,
			Message:        formatMessage(oom),
			FirstTimestamp: t,
			LastTimestamp:  t,
			Count:          1,
			Type:           "Warning",
			Source: v1.EventSource{
				Component: "kube-oom-monitor",
				Host:      *nodeName,
			},
		}

		event, err = clientset.CoreV1().Events(metav1.NamespaceDefault).Create(context.TODO(), event, metav1.CreateOptions{})
		if err != nil {
			klog.Errorf("Unable to write event: '%v'", err)
		}
	}
}

func formatMessage(oom *oomparser.OomInstance) string {
	return fmt.Sprintf("pid:%v\nproc:%v\ntaskcg:%v\noomcg:%v",
		oom.Pid, oom.ProcessName, oom.ContainerName, oom.VictimContainerName)
}

// https://github.com/smpio/kube-oom-monitor/issues/1
func syncClock() (*time.Duration, error) {
	t := time.Now()
	msgText := fmt.Sprintf("current_time_unix_nano:%d", t.UnixNano())

	f, err := os.OpenFile("/dev/kmsg", os.O_WRONLY, 0)
	if err != nil {
		return nil, err
	}
	_, err = f.WriteString(msgText + "\n")
	if err != nil {
		return nil, err
	}
	f.Close()

	parser, err := kmsgparser.NewParser()
	if err != nil {
		return nil, err
	}
	kmsgEntries := parser.Parse()
	defer parser.Close()

	for msg := range kmsgEntries {
		if strings.HasPrefix(msg.Message, msgText) {
			drift := t.Sub(msg.Timestamp)
			return &drift, nil
		}
	}

	return nil, errors.New("can't sync clocks: message not found in kernel log buffer")
}
