/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package framework

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	runtimeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/kubefed/test/common"
)

// WaitForObject waits for object to match the desired status.
func WaitForObject(tl common.TestLogger, namespace, name string, objectGetter func(namespace, name string) (runtimeclient.Object, error), desired runtimeclient.Object, equivalent func(actual, desired runtimeclient.Object) bool) {
	var actual runtimeclient.Object
	interval := PollInterval
	timeout := TestContext.SingleCallTimeout
	err := wait.PollUntilContextTimeout(context.Background(), interval, timeout, true, func(ctx context.Context) (exist bool, err error) {
		actual, err = objectGetter(namespace, name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}

		return equivalent(actual, desired), nil
	})
	if err != nil {
		tl.Fatalf("Timedout waiting for desired state, \ndesired: %#v\nactual:  %#v", desired, actual)
	}
}

// WaitUntilLogStreamContains waits for the given stream to contain the
// substring until the end of the stream or timeout.
func WaitUntilLogStreamContains(tl common.TestLogger, stream io.ReadCloser, substr string) bool {
	scanner := bufio.NewScanner(stream)
	done := make(chan bool, 1)
	go func() {
		for scanner.Scan() {
			line := scanner.Text()
			tl.Log(line)
			if strings.Contains(line, substr) {
				done <- true
				return
			}
		}
		done <- false
	}()

	select {
	case result := <-done:
		return result
	case <-time.After(TestContext.SingleCallTimeout):
		tl.Fatalf("Timeout waiting for stream to contain substring = %q", substr)
	}
	return false
}
