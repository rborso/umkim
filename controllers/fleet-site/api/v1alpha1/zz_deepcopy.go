package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (in *FleetSite) DeepCopyInto(out *FleetSite) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	if in.Spec.ClusterCapabilities != nil {
		out.Spec.ClusterCapabilities = make(map[string]bool, len(in.Spec.ClusterCapabilities))
		for k, v := range in.Spec.ClusterCapabilities {
			out.Spec.ClusterCapabilities[k] = v
		}
	}
	if in.Spec.Storage != nil {
		in, out := &in.Spec.Storage, &out.Spec.Storage
		*out = new(ClusterStorageSpec)
		**out = **in
		if (*in).NFS != nil {
			nfs := *(*in).NFS
			(*out).NFS = &nfs
		}
	}
	in.Status.DeepCopyInto(&out.Status)
}

func (in *FleetSite) DeepCopy() *FleetSite {
	if in == nil {
		return nil
	}
	out := new(FleetSite)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetSite) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetSiteStatus) DeepCopyInto(out *FleetSiteStatus) {
	*out = *in
	if in.EnrolledAt != nil {
		t := *in.EnrolledAt
		out.EnrolledAt = &t
	}
	if in.ValidationErrors != nil {
		out.ValidationErrors = append([]string(nil), in.ValidationErrors...)
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *FleetSiteList) DeepCopyInto(out *FleetSiteList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]FleetSite, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *FleetSiteList) DeepCopy() *FleetSiteList {
	if in == nil {
		return nil
	}
	out := new(FleetSiteList)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetSiteList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetClusterJoin) DeepCopyInto(out *FleetClusterJoin) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	if in.Spec.ClusterCapabilities != nil {
		out.Spec.ClusterCapabilities = make(map[string]bool, len(in.Spec.ClusterCapabilities))
		for k, v := range in.Spec.ClusterCapabilities {
			out.Spec.ClusterCapabilities[k] = v
		}
	}
	if in.Spec.Storage != nil {
		in, out := &in.Spec.Storage, &out.Spec.Storage
		*out = new(ClusterStorageSpec)
		**out = **in
		if (*in).NFS != nil {
			nfs := *(*in).NFS
			(*out).NFS = &nfs
		}
	}
	if in.Spec.Nodes != nil {
		out.Spec.Nodes = append([]FleetClusterJoinNodeSpec(nil), in.Spec.Nodes...)
	}
	in.Status.DeepCopyInto(&out.Status)
}

func (in *FleetIngressAuth) DeepCopyInto(out *FleetIngressAuth) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	if in.Spec.AccessDomains != nil {
		out.Spec.AccessDomains = append([]string(nil), in.Spec.AccessDomains...)
	}
	in.Status.DeepCopyInto(&out.Status)
}

func (in *FleetIngressAuth) DeepCopy() *FleetIngressAuth {
	if in == nil {
		return nil
	}
	out := new(FleetIngressAuth)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetIngressAuth) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetIngressAuthStatus) DeepCopyInto(out *FleetIngressAuthStatus) {
	*out = *in
	if in.Clusters != nil {
		out.Clusters = append([]FleetIngressAuthClusterStatus(nil), in.Clusters...)
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *FleetIngressAuthList) DeepCopyInto(out *FleetIngressAuthList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]FleetIngressAuth, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *FleetIngressAuthList) DeepCopy() *FleetIngressAuthList {
	if in == nil {
		return nil
	}
	out := new(FleetIngressAuthList)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetIngressAuthList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetAcme) DeepCopyInto(out *FleetAcme) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

func (in *FleetAcme) DeepCopy() *FleetAcme {
	if in == nil {
		return nil
	}
	out := new(FleetAcme)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetAcme) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetAcmeStatus) DeepCopyInto(out *FleetAcmeStatus) {
	*out = *in
	if in.Clusters != nil {
		out.Clusters = append([]FleetAcmeClusterStatus(nil), in.Clusters...)
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *FleetAcmeList) DeepCopyInto(out *FleetAcmeList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]FleetAcme, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *FleetAcmeList) DeepCopy() *FleetAcmeList {
	if in == nil {
		return nil
	}
	out := new(FleetAcmeList)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetAcmeList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetClusterJoin) DeepCopy() *FleetClusterJoin {
	if in == nil {
		return nil
	}
	out := new(FleetClusterJoin)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetClusterJoin) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetClusterJoinStatus) DeepCopyInto(out *FleetClusterJoinStatus) {
	*out = *in
	if in.EnrolledAt != nil {
		t := *in.EnrolledAt
		out.EnrolledAt = &t
	}
	if in.ValidationErrors != nil {
		out.ValidationErrors = append([]string(nil), in.ValidationErrors...)
	}
	if in.Nodes != nil {
		out.Nodes = append([]FleetClusterJoinNodeStatus(nil), in.Nodes...)
	}
	if in.Providers != nil {
		out.Providers = make(map[string]ClusterProviderStatus, len(in.Providers))
		for k, v := range in.Providers {
			out.Providers[k] = v
		}
	}
	if in.DeletionStartedAt != nil {
		t := *in.DeletionStartedAt
		out.DeletionStartedAt = &t
	}
	if in.IngressAuth.AccessDomains != nil {
		out.IngressAuth.AccessDomains = append([]string(nil), in.IngressAuth.AccessDomains...)
	}
	if in.NodeOperation != nil {
		in, out := &in.NodeOperation, &out.NodeOperation
		*out = new(ClusterNodeOperationStatus)
		(*in).DeepCopyInto(*out)
	}
	if in.NodeOperationHistory != nil {
		in, out := &in.NodeOperationHistory, &out.NodeOperationHistory
		*out = make([]ClusterNodeOperationRecord, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

func (in *ClusterNodeOperationStatus) DeepCopyInto(out *ClusterNodeOperationStatus) {
	*out = *in
	if in.StartedAt != nil {
		t := *in.StartedAt
		out.StartedAt = &t
	}
}

func (in *ClusterNodeOperationStatus) DeepCopy() *ClusterNodeOperationStatus {
	if in == nil {
		return nil
	}
	out := new(ClusterNodeOperationStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *ClusterNodeOperationRecord) DeepCopyInto(out *ClusterNodeOperationRecord) {
	*out = *in
	if in.StartedAt != nil {
		t := *in.StartedAt
		out.StartedAt = &t
	}
	if in.FinishedAt != nil {
		t := *in.FinishedAt
		out.FinishedAt = &t
	}
}

func (in *ClusterNodeOperationRecord) DeepCopy() *ClusterNodeOperationRecord {
	if in == nil {
		return nil
	}
	out := new(ClusterNodeOperationRecord)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetClusterJoinList) DeepCopyInto(out *FleetClusterJoinList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]FleetClusterJoin, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *FleetClusterJoinList) DeepCopy() *FleetClusterJoinList {
	if in == nil {
		return nil
	}
	out := new(FleetClusterJoinList)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetClusterJoinList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetInstanceBackup) DeepCopyInto(out *FleetInstanceBackup) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

func (in *FleetInstanceBackup) DeepCopy() *FleetInstanceBackup {
	if in == nil {
		return nil
	}
	out := new(FleetInstanceBackup)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetInstanceBackup) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetInstanceBackupStatus) DeepCopyInto(out *FleetInstanceBackupStatus) {
	*out = *in
	if in.StartedAt != nil {
		in, out := &in.StartedAt, &out.StartedAt
		*out = (*in).DeepCopy()
	}
	if in.CompletedAt != nil {
		in, out := &in.CompletedAt, &out.CompletedAt
		*out = (*in).DeepCopy()
	}
}

func (in *FleetInstanceBackupList) DeepCopyInto(out *FleetInstanceBackupList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]FleetInstanceBackup, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *FleetInstanceBackupList) DeepCopy() *FleetInstanceBackupList {
	if in == nil {
		return nil
	}
	out := new(FleetInstanceBackupList)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetInstanceBackupList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetInstanceRestore) DeepCopyInto(out *FleetInstanceRestore) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *FleetInstanceRestore) DeepCopy() *FleetInstanceRestore {
	if in == nil {
		return nil
	}
	out := new(FleetInstanceRestore)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetInstanceRestore) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetInstanceRestoreSpec) DeepCopyInto(out *FleetInstanceRestoreSpec) {
	*out = *in
	if in.Rebind != nil {
		in, out := &in.Rebind, &out.Rebind
		*out = new(FleetInstanceRestoreRebindSpec)
		(*in).DeepCopyInto(*out)
	}
	if in.StorageClassHints != nil {
		in, out := &in.StorageClassHints, &out.StorageClassHints
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

func (in *FleetInstanceRestoreRebindSpec) DeepCopyInto(out *FleetInstanceRestoreRebindSpec) {
	*out = *in
	if in.UserValuePatches != nil {
		in, out := &in.UserValuePatches, &out.UserValuePatches
		*out = make([]UserValuePatch, len(*in))
		copy(*out, *in)
	}
}

func (in *FleetInstanceRestoreStatus) DeepCopyInto(out *FleetInstanceRestoreStatus) {
	*out = *in
	if in.StartedAt != nil {
		in, out := &in.StartedAt, &out.StartedAt
		*out = (*in).DeepCopy()
	}
	if in.CompletedAt != nil {
		in, out := &in.CompletedAt, &out.CompletedAt
		*out = (*in).DeepCopy()
	}
}

func (in *FleetInstanceRestoreList) DeepCopyInto(out *FleetInstanceRestoreList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]FleetInstanceRestore, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *FleetInstanceRestoreList) DeepCopy() *FleetInstanceRestoreList {
	if in == nil {
		return nil
	}
	out := new(FleetInstanceRestoreList)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetInstanceRestoreList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetLLM) DeepCopyInto(out *FleetLLM) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *FleetLLM) DeepCopy() *FleetLLM {
	if in == nil {
		return nil
	}
	out := new(FleetLLM)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetLLM) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetLLMSpec) DeepCopyInto(out *FleetLLMSpec) {
	*out = *in
	if in.Backends != nil {
		out.Backends = make([]FleetLLMBackend, len(in.Backends))
		copy(out.Backends, in.Backends)
	}
}

func (in *FleetLLMStatus) DeepCopyInto(out *FleetLLMStatus) {
	*out = *in
	out.Hub = in.Hub
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *FleetLLMList) DeepCopyInto(out *FleetLLMList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]FleetLLM, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *FleetLLMList) DeepCopy() *FleetLLMList {
	if in == nil {
		return nil
	}
	out := new(FleetLLMList)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetLLMList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetApp) DeepCopyInto(out *FleetApp) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

func (in *FleetApp) DeepCopy() *FleetApp {
	if in == nil {
		return nil
	}
	out := new(FleetApp)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetApp) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetAppStatus) DeepCopyInto(out *FleetAppStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *FleetAppList) DeepCopyInto(out *FleetAppList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]FleetApp, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *FleetAppList) DeepCopy() *FleetAppList {
	if in == nil {
		return nil
	}
	out := new(FleetAppList)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetAppList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetChart) DeepCopyInto(out *FleetChart) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

func (in *FleetChart) DeepCopy() *FleetChart {
	if in == nil {
		return nil
	}
	out := new(FleetChart)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetChart) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *FleetChartStatus) DeepCopyInto(out *FleetChartStatus) {
	*out = *in
	if in.Versions != nil {
		out.Versions = append([]string(nil), in.Versions...)
	}
	if in.ImagesCopied != nil {
		out.ImagesCopied = make([]FleetChartCopiedImage, len(in.ImagesCopied))
		copy(out.ImagesCopied, in.ImagesCopied)
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *FleetChartList) DeepCopyInto(out *FleetChartList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]FleetChart, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *FleetChartList) DeepCopy() *FleetChartList {
	if in == nil {
		return nil
	}
	out := new(FleetChartList)
	in.DeepCopyInto(out)
	return out
}

func (in *FleetChartList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
