/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"os"
	"os/exec"
)

// Diff prints the unified diff of the two provided byte slices
// using the unix diff command.
func Diff(left, right []byte) error {
	lf, err := os.CreateTemp("/tmp", "actual-file-")
	if err != nil {
		return err
	}
	defer lf.Close()
	defer os.Remove(lf.Name())

	rf, err := os.CreateTemp("/tmp", "expected-file-")
	if err != nil {
		return err
	}
	defer rf.Close()
	defer os.Remove(rf.Name())

	_, err = lf.Write(left)
	if err != nil {
		return err
	}
	lf.Close()

	_, err = rf.Write(right)
	if err != nil {
		return err
	}
	rf.Close()

	cmd := exec.Command("/usr/bin/diff", "-u", lf.Name(), rf.Name())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

	return nil
}
