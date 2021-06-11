package main

import (
	"context"
	"flag"
	"fmt"

	"github.com/google/cadvisor/utils/oomparser"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
)

func main() {
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

	outStream := make(chan *oomparser.OomInstance)
	oomLog, err := oomparser.New()
	if err != nil {
		klog.Fatalf("Couldn't make a new oomparser. %v", err)
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

	go oomLog.StreamOoms(outStream)
	klog.Infoln("Reading the OOM stream")

	for oom := range outStream {
		klog.Infof("OOM: %+v", oom)

		t := metav1.Time{Time: oom.TimeOfDeath}

		event := &v1.Event{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("%v.%x", nodeName, t.UnixNano()),
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
		}

		event, err = clientset.CoreV1().Events("").Create(context.TODO(), event, metav1.CreateOptions{})
		if err != nil {
			klog.Errorf("Unable to write event: '%v'", err)
		}
	}
}

func formatMessage(oom *oomparser.OomInstance) string {
	return fmt.Sprintf("pid:%v\nproc:%v\ntaskcg:%v\noomcg:%v",
		oom.Pid, oom.ProcessName, oom.ContainerName, oom.VictimContainerName)
}
