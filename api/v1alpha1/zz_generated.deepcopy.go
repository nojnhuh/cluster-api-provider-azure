//go:build !ignore_autogenerated

/*
Copyright 2024.

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOClusterTemplateResource) DeepCopyInto(out *ASOClusterTemplateResource) {
	*out = *in
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOClusterTemplateResource.
func (in *ASOClusterTemplateResource) DeepCopy() *ASOClusterTemplateResource {
	if in == nil {
		return nil
	}
	out := new(ASOClusterTemplateResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedCluster) DeepCopyInto(out *ASOManagedCluster) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedCluster.
func (in *ASOManagedCluster) DeepCopy() *ASOManagedCluster {
	if in == nil {
		return nil
	}
	out := new(ASOManagedCluster)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ASOManagedCluster) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedClusterList) DeepCopyInto(out *ASOManagedClusterList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ASOManagedCluster, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedClusterList.
func (in *ASOManagedClusterList) DeepCopy() *ASOManagedClusterList {
	if in == nil {
		return nil
	}
	out := new(ASOManagedClusterList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ASOManagedClusterList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedClusterSpec) DeepCopyInto(out *ASOManagedClusterSpec) {
	*out = *in
	in.ASOManagedClusterTemplateResourceSpec.DeepCopyInto(&out.ASOManagedClusterTemplateResourceSpec)
	out.ControlPlaneEndpoint = in.ControlPlaneEndpoint
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedClusterSpec.
func (in *ASOManagedClusterSpec) DeepCopy() *ASOManagedClusterSpec {
	if in == nil {
		return nil
	}
	out := new(ASOManagedClusterSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedClusterStatus) DeepCopyInto(out *ASOManagedClusterStatus) {
	*out = *in
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = make([]ResourceStatus, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedClusterStatus.
func (in *ASOManagedClusterStatus) DeepCopy() *ASOManagedClusterStatus {
	if in == nil {
		return nil
	}
	out := new(ASOManagedClusterStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedClusterTemplate) DeepCopyInto(out *ASOManagedClusterTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedClusterTemplate.
func (in *ASOManagedClusterTemplate) DeepCopy() *ASOManagedClusterTemplate {
	if in == nil {
		return nil
	}
	out := new(ASOManagedClusterTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ASOManagedClusterTemplate) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedClusterTemplateList) DeepCopyInto(out *ASOManagedClusterTemplateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ASOManagedClusterTemplate, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedClusterTemplateList.
func (in *ASOManagedClusterTemplateList) DeepCopy() *ASOManagedClusterTemplateList {
	if in == nil {
		return nil
	}
	out := new(ASOManagedClusterTemplateList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ASOManagedClusterTemplateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedClusterTemplateResourceSpec) DeepCopyInto(out *ASOManagedClusterTemplateResourceSpec) {
	*out = *in
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = make([]runtime.RawExtension, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedClusterTemplateResourceSpec.
func (in *ASOManagedClusterTemplateResourceSpec) DeepCopy() *ASOManagedClusterTemplateResourceSpec {
	if in == nil {
		return nil
	}
	out := new(ASOManagedClusterTemplateResourceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedClusterTemplateSpec) DeepCopyInto(out *ASOManagedClusterTemplateSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedClusterTemplateSpec.
func (in *ASOManagedClusterTemplateSpec) DeepCopy() *ASOManagedClusterTemplateSpec {
	if in == nil {
		return nil
	}
	out := new(ASOManagedClusterTemplateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedControlPlane) DeepCopyInto(out *ASOManagedControlPlane) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedControlPlane.
func (in *ASOManagedControlPlane) DeepCopy() *ASOManagedControlPlane {
	if in == nil {
		return nil
	}
	out := new(ASOManagedControlPlane)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ASOManagedControlPlane) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedControlPlaneList) DeepCopyInto(out *ASOManagedControlPlaneList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ASOManagedControlPlane, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedControlPlaneList.
func (in *ASOManagedControlPlaneList) DeepCopy() *ASOManagedControlPlaneList {
	if in == nil {
		return nil
	}
	out := new(ASOManagedControlPlaneList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ASOManagedControlPlaneList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedControlPlaneResource) DeepCopyInto(out *ASOManagedControlPlaneResource) {
	*out = *in
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedControlPlaneResource.
func (in *ASOManagedControlPlaneResource) DeepCopy() *ASOManagedControlPlaneResource {
	if in == nil {
		return nil
	}
	out := new(ASOManagedControlPlaneResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedControlPlaneSpec) DeepCopyInto(out *ASOManagedControlPlaneSpec) {
	*out = *in
	in.ASOManagedControlPlaneTemplateResourceSpec.DeepCopyInto(&out.ASOManagedControlPlaneTemplateResourceSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedControlPlaneSpec.
func (in *ASOManagedControlPlaneSpec) DeepCopy() *ASOManagedControlPlaneSpec {
	if in == nil {
		return nil
	}
	out := new(ASOManagedControlPlaneSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedControlPlaneStatus) DeepCopyInto(out *ASOManagedControlPlaneStatus) {
	*out = *in
	out.ControlPlaneEndpoint = in.ControlPlaneEndpoint
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = make([]ResourceStatus, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedControlPlaneStatus.
func (in *ASOManagedControlPlaneStatus) DeepCopy() *ASOManagedControlPlaneStatus {
	if in == nil {
		return nil
	}
	out := new(ASOManagedControlPlaneStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedControlPlaneTemplate) DeepCopyInto(out *ASOManagedControlPlaneTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedControlPlaneTemplate.
func (in *ASOManagedControlPlaneTemplate) DeepCopy() *ASOManagedControlPlaneTemplate {
	if in == nil {
		return nil
	}
	out := new(ASOManagedControlPlaneTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ASOManagedControlPlaneTemplate) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedControlPlaneTemplateList) DeepCopyInto(out *ASOManagedControlPlaneTemplateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ASOManagedControlPlaneTemplate, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedControlPlaneTemplateList.
func (in *ASOManagedControlPlaneTemplateList) DeepCopy() *ASOManagedControlPlaneTemplateList {
	if in == nil {
		return nil
	}
	out := new(ASOManagedControlPlaneTemplateList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ASOManagedControlPlaneTemplateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedControlPlaneTemplateResourceSpec) DeepCopyInto(out *ASOManagedControlPlaneTemplateResourceSpec) {
	*out = *in
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = make([]runtime.RawExtension, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedControlPlaneTemplateResourceSpec.
func (in *ASOManagedControlPlaneTemplateResourceSpec) DeepCopy() *ASOManagedControlPlaneTemplateResourceSpec {
	if in == nil {
		return nil
	}
	out := new(ASOManagedControlPlaneTemplateResourceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedControlPlaneTemplateSpec) DeepCopyInto(out *ASOManagedControlPlaneTemplateSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedControlPlaneTemplateSpec.
func (in *ASOManagedControlPlaneTemplateSpec) DeepCopy() *ASOManagedControlPlaneTemplateSpec {
	if in == nil {
		return nil
	}
	out := new(ASOManagedControlPlaneTemplateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedMachinePool) DeepCopyInto(out *ASOManagedMachinePool) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedMachinePool.
func (in *ASOManagedMachinePool) DeepCopy() *ASOManagedMachinePool {
	if in == nil {
		return nil
	}
	out := new(ASOManagedMachinePool)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ASOManagedMachinePool) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedMachinePoolList) DeepCopyInto(out *ASOManagedMachinePoolList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ASOManagedMachinePool, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedMachinePoolList.
func (in *ASOManagedMachinePoolList) DeepCopy() *ASOManagedMachinePoolList {
	if in == nil {
		return nil
	}
	out := new(ASOManagedMachinePoolList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ASOManagedMachinePoolList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedMachinePoolResource) DeepCopyInto(out *ASOManagedMachinePoolResource) {
	*out = *in
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedMachinePoolResource.
func (in *ASOManagedMachinePoolResource) DeepCopy() *ASOManagedMachinePoolResource {
	if in == nil {
		return nil
	}
	out := new(ASOManagedMachinePoolResource)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedMachinePoolSpec) DeepCopyInto(out *ASOManagedMachinePoolSpec) {
	*out = *in
	in.ASOManagedMachinePoolTemplateResourceSpec.DeepCopyInto(&out.ASOManagedMachinePoolTemplateResourceSpec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedMachinePoolSpec.
func (in *ASOManagedMachinePoolSpec) DeepCopy() *ASOManagedMachinePoolSpec {
	if in == nil {
		return nil
	}
	out := new(ASOManagedMachinePoolSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedMachinePoolStatus) DeepCopyInto(out *ASOManagedMachinePoolStatus) {
	*out = *in
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = make([]ResourceStatus, len(*in))
		copy(*out, *in)
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedMachinePoolStatus.
func (in *ASOManagedMachinePoolStatus) DeepCopy() *ASOManagedMachinePoolStatus {
	if in == nil {
		return nil
	}
	out := new(ASOManagedMachinePoolStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedMachinePoolTemplate) DeepCopyInto(out *ASOManagedMachinePoolTemplate) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedMachinePoolTemplate.
func (in *ASOManagedMachinePoolTemplate) DeepCopy() *ASOManagedMachinePoolTemplate {
	if in == nil {
		return nil
	}
	out := new(ASOManagedMachinePoolTemplate)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ASOManagedMachinePoolTemplate) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedMachinePoolTemplateList) DeepCopyInto(out *ASOManagedMachinePoolTemplateList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]ASOManagedMachinePoolTemplate, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedMachinePoolTemplateList.
func (in *ASOManagedMachinePoolTemplateList) DeepCopy() *ASOManagedMachinePoolTemplateList {
	if in == nil {
		return nil
	}
	out := new(ASOManagedMachinePoolTemplateList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *ASOManagedMachinePoolTemplateList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedMachinePoolTemplateResourceSpec) DeepCopyInto(out *ASOManagedMachinePoolTemplateResourceSpec) {
	*out = *in
	if in.ProviderIDList != nil {
		in, out := &in.ProviderIDList, &out.ProviderIDList
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = make([]runtime.RawExtension, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedMachinePoolTemplateResourceSpec.
func (in *ASOManagedMachinePoolTemplateResourceSpec) DeepCopy() *ASOManagedMachinePoolTemplateResourceSpec {
	if in == nil {
		return nil
	}
	out := new(ASOManagedMachinePoolTemplateResourceSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ASOManagedMachinePoolTemplateSpec) DeepCopyInto(out *ASOManagedMachinePoolTemplateSpec) {
	*out = *in
	in.Template.DeepCopyInto(&out.Template)
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ASOManagedMachinePoolTemplateSpec.
func (in *ASOManagedMachinePoolTemplateSpec) DeepCopy() *ASOManagedMachinePoolTemplateSpec {
	if in == nil {
		return nil
	}
	out := new(ASOManagedMachinePoolTemplateSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ResourceStatus) DeepCopyInto(out *ResourceStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ResourceStatus.
func (in *ResourceStatus) DeepCopy() *ResourceStatus {
	if in == nil {
		return nil
	}
	out := new(ResourceStatus)
	in.DeepCopyInto(out)
	return out
}