package utils

import (
	"bytes"
	"context"
	"fmt"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
	"time"
)

type PodExecutor struct {
	config     *rest.Config
	restClient kubernetes.Interface

}

func NewPodExecutor(config *rest.Config) (*PodExecutor, error) {
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Wrap(err, "Failed to initialize PodExecutor")
	}

	return &PodExecutor{
		config: config,
		restClient: client,
	}, nil
}

func (pe *PodExecutor) Exec(inputPod *corev1.Pod, command []string) (string, error) {
	podClient := pe.restClient.CoreV1().Pods(inputPod.GetNamespace())

	err := wait.Poll(500*time.Millisecond, 5*time.Minute, func() (bool, error) {
		pod, errPod := podClient.Get(context.TODO(),inputPod.GetName(), v1.GetOptions{})
		if errPod != nil {
			// This could be a connection error so we want to retry.
			return false, nil
		}
		for _, condition := range pod.Status.Conditions {
			if condition.Type == corev1.PodReady {
				if condition.Status == corev1.ConditionTrue {
					return true, nil
				}
			}
		}
		return false, nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to check the pod: %v", err)
	}

	req := pe.restClient.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(inputPod.GetName()).
		Namespace(inputPod.GetNamespace()).
		SubResource("exec")

	option := &corev1.PodExecOptions{
		Command: command,
		Stdin:   false,
		Stdout:  true,
		Stderr:  true,
		TTY:     true,
	}
	req.VersionedParams(
		option,
		scheme.ParameterCodec,
	)

	exec, err := remotecommand.NewSPDYExecutor(pe.config, "POST", req.URL())
	if err != nil {
		return "", errors.Wrap(err, "failed to init the executor")
	}

	var (
		execOut bytes.Buffer
		execErr bytes.Buffer
	)

	err = exec.Stream(remotecommand.StreamOptions{
		Stdout: &execOut,
		Stderr: &execErr,
		Tty:    false,
	})

	if err != nil {
		return "", errors.Wrap(err, "could not execute the command")
	}

	if execErr.Len() > 0 {
		return "", errors.Wrapf(err, "error executing the command, maybe not available in the version: %s", execErr.String())
	}

	return execOut.String(), nil
}
