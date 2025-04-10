/*
Copyright 2025 The Kubernetes Authors.

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

package v1alpha1

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

func TestAzureASOMachineWebhookValidateCreate(t *testing.T) {
	tests := []struct {
		name       string
		asoMachine *AzureASOMachine
		matchErrs  types.GomegaMatcher
	}{
		{
			name:       "empty",
			asoMachine: &AzureASOMachine{},
			matchErrs:  Not(HaveOccurred()),
		},
	}

	ctx := context.Background()

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := NewGomegaWithT(t)
			a := &azureASOMachineWebhook{}

			if test.matchErrs == nil {
				test.matchErrs = BeEmpty()
			}

			actualWarnings, actualErrs := a.ValidateCreate(ctx, test.asoMachine)
			g.Expect(actualWarnings).To(BeEmpty())
			g.Expect(actualErrs).To(test.matchErrs)
		})
	}
}
