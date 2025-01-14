// +build varlink

package varlinkapi

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/containers/buildah"
	"github.com/containers/libpod/cmd/podman/shared"
	"github.com/containers/libpod/cmd/podman/varlink"
	"github.com/containers/libpod/libpod"
	"github.com/containers/libpod/libpod/define"
	"github.com/containers/storage/pkg/archive"
)

// getContext returns a non-nil, empty context
func getContext() context.Context {
	return context.TODO()
}

func makeListContainer(containerID string, batchInfo shared.BatchContainerStruct) iopodman.Container {
	var (
		mounts []iopodman.ContainerMount
		ports  []iopodman.ContainerPortMappings
	)
	ns := shared.GetNamespaces(batchInfo.Pid)

	for _, mount := range batchInfo.ConConfig.Spec.Mounts {
		m := iopodman.ContainerMount{
			Destination: mount.Destination,
			Type:        mount.Type,
			Source:      mount.Source,
			Options:     mount.Options,
		}
		mounts = append(mounts, m)
	}

	for _, pm := range batchInfo.ConConfig.PortMappings {
		p := iopodman.ContainerPortMappings{
			Host_port:      strconv.Itoa(int(pm.HostPort)),
			Host_ip:        pm.HostIP,
			Protocol:       pm.Protocol,
			Container_port: strconv.Itoa(int(pm.ContainerPort)),
		}
		ports = append(ports, p)

	}

	// If we find this needs to be done for other container endpoints, we should
	// convert this to a separate function or a generic map from struct function.
	namespace := iopodman.ContainerNameSpace{
		User:   ns.User,
		Uts:    ns.UTS,
		Pidns:  ns.PIDNS,
		Pid:    ns.PID,
		Cgroup: ns.Cgroup,
		Net:    ns.NET,
		Mnt:    ns.MNT,
		Ipc:    ns.IPC,
	}

	lc := iopodman.Container{
		Id:               containerID,
		Image:            batchInfo.ConConfig.RootfsImageName,
		Imageid:          batchInfo.ConConfig.RootfsImageID,
		Command:          batchInfo.ConConfig.Spec.Process.Args,
		Createdat:        batchInfo.ConConfig.CreatedTime.Format(time.RFC3339),
		Runningfor:       time.Since(batchInfo.ConConfig.CreatedTime).String(),
		Status:           batchInfo.ConState.String(),
		Ports:            ports,
		Names:            batchInfo.ConConfig.Name,
		Labels:           batchInfo.ConConfig.Labels,
		Mounts:           mounts,
		Containerrunning: batchInfo.ConState == define.ContainerStateRunning,
		Namespaces:       namespace,
	}
	if batchInfo.Size != nil {
		lc.Rootfssize = batchInfo.Size.RootFsSize
		lc.Rwsize = batchInfo.Size.RwSize
	}
	return lc
}

func makeListPodContainers(containerID string, batchInfo shared.BatchContainerStruct) iopodman.ListPodContainerInfo {
	lc := iopodman.ListPodContainerInfo{
		Id:     containerID,
		Status: batchInfo.ConState.String(),
		Name:   batchInfo.ConConfig.Name,
	}
	return lc
}

func makeListPod(pod *libpod.Pod, batchInfo shared.PsOptions) (iopodman.ListPodData, error) {
	var listPodsContainers []iopodman.ListPodContainerInfo
	var errPodData = iopodman.ListPodData{}
	status, err := shared.GetPodStatus(pod)
	if err != nil {
		return errPodData, err
	}
	containers, err := pod.AllContainers()
	if err != nil {
		return errPodData, err
	}
	for _, ctr := range containers {
		batchInfo, err := shared.BatchContainerOp(ctr, batchInfo)
		if err != nil {
			return errPodData, err
		}

		listPodsContainers = append(listPodsContainers, makeListPodContainers(ctr.ID(), batchInfo))
	}
	listPod := iopodman.ListPodData{
		Createdat:          pod.CreatedTime().Format(time.RFC3339),
		Id:                 pod.ID(),
		Name:               pod.Name(),
		Status:             status,
		Cgroup:             pod.CgroupParent(),
		Numberofcontainers: strconv.Itoa(len(listPodsContainers)),
		Containersinfo:     listPodsContainers,
	}
	return listPod, nil
}

func handlePodCall(call iopodman.VarlinkCall, pod *libpod.Pod, ctrErrs map[string]error, err error) error {
	if err != nil && ctrErrs == nil {
		return call.ReplyErrorOccurred(err.Error())
	}
	if ctrErrs != nil {
		containerErrs := make([]iopodman.PodContainerErrorData, len(ctrErrs))
		for ctr, reason := range ctrErrs {
			ctrErr := iopodman.PodContainerErrorData{Containerid: ctr, Reason: reason.Error()}
			containerErrs = append(containerErrs, ctrErr)
		}
		return call.ReplyPodContainerError(pod.ID(), containerErrs)
	}

	return nil
}

func stringCompressionToArchiveType(s string) archive.Compression {
	switch strings.ToUpper(s) {
	case "BZIP2":
		return archive.Bzip2
	case "GZIP":
		return archive.Gzip
	case "XZ":
		return archive.Xz
	}
	return archive.Uncompressed
}

func stringPullPolicyToType(s string) buildah.PullPolicy {
	switch strings.ToUpper(s) {
	case "PULLIFMISSING":
		return buildah.PullIfMissing
	case "PULLALWAYS":
		return buildah.PullAlways
	case "PULLNEVER":
		return buildah.PullNever
	}
	return buildah.PullIfMissing
}

func derefBool(inBool *bool) bool {
	if inBool == nil {
		return false
	}
	return *inBool
}

func derefString(in *string) string {
	if in == nil {
		return ""
	}
	return *in
}

func makePsOpts(inOpts iopodman.PsOpts) shared.PsOptions {
	last := 0
	if inOpts.Last != nil {
		lastT := *inOpts.Last
		last = int(lastT)
	}
	return shared.PsOptions{
		All:       inOpts.All,
		Last:      last,
		Latest:    derefBool(inOpts.Latest),
		NoTrunc:   derefBool(inOpts.NoTrunc),
		Pod:       derefBool(inOpts.Pod),
		Size:      true,
		Sort:      derefString(inOpts.Sort),
		Namespace: true,
		Sync:      derefBool(inOpts.Sync),
	}
}
