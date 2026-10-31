// dispatch.go: action-name -> runnable resolution, split into
// per-family tables. dispatch is a thin router; every action name
// lives in exactly one family, so probe order is irrelevant.
package main

// dispatch resolves an action name to its runnable; nil when the
// action is unknown (e.g. "start" — handled by the runActions loop).
func dispatch(act string) func() int {
	for _, sub := range []func(string) func() int{
		dispatchLifecycle,
		dispatchMonitor,
		dispatchAdmin,
		dispatchBackup,
		dispatchSnapshotDisk,
		dispatchInfo,
		dispatchOps,
		dispatchViewers,
	} {
		if fn := sub(act); fn != nil {
			return fn
		}
	}
	return nil
}

// dispatchLifecycle: power-state and status actions.
func dispatchLifecycle(act string) func() int {
	switch act {
	case "status_vm":
		return actStatus
	case "list_vms":
		return actListVMs
	case "pause_vm":
		return actPause
	case "resume_vm":
		return actResume
	case "shutdown_vm":
		return actShutdown
	case "reset_vm":
		return actReset
	case "kill_vm":
		return func() int { return actStop(true) }
	case "stop_vm":
		return func() int { return actStop(false) }
	}
	return nil
}

// dispatchMonitor: guest-interaction actions (QMP/HMP driven).
func dispatchMonitor(act string) func() int {
	switch act {
	case "monitor_cmd":
		return actMonitorCmd
	case "send_keys":
		return actSendKeys
	case "type_text":
		return actType
	case "screenshot_vm":
		return actScreenshot
	}
	return nil
}

// dispatchAdmin: VM admin ops delegated to quickemu-vm (delete,
// clone, rename, move, autostart, shortcuts, vfio, plans) plus the
// host-side kvm ignore_msrs persist.
func dispatchAdmin(act string) func() int {
	switch act {
	case "delete_vm":
		return func() int {
			a := []string{"delete", vmdir()}
			if trashFlag {
				a = append(a, "--trash")
			}
			if extra["plan-id"] != "" {
				a = append(a, "--plan-id="+extra["plan-id"])
			}
			return vmOp(a[0], a[1:]...)()
		}
	case "clone_vm":
		return func() int {
			a := []string{"clone", vmdir(), extra["clone"]}
			if extra["full"] != "" {
				a = append(a, "--full")
			}
			return vmOp(a[0], a[1:]...)()
		}
	case "linked_clone_vm":
		return vmOp("linked-clone", vmdir(), extra["linked-clone"])
	case "rename_vm":
		return vmOp("rename", vmdir(), extra["rename"])
	case "move_vm":
		return vmOp("move", vmdir(), extra["move"])
	case "vfio_scan":
		return vmOp("vfio-scan", vmdir())
	case "autostart_vm":
		return vmOp("autostart", vmdir())
	case "shortcut_create":
		return func() int {
			a := []string{"shortcut", vmdir()}
			return vmOp(a[0], append(a[1:], shortcutOpts...)...)()
		}
	case "delete_shortcut":
		return vmOp("shortcut-delete", vmdir())
	case "delete_plan":
		return vmOp("delete-plan", vmdir(), "--format", outFmt)
	case "ignore_msrs_always":
		return actIgnoreMSRsAlways
	}
	return nil
}

// dispatchBackup: export/import, backup and trash ops.
func dispatchBackup(act string) func() int {
	switch act {
	case "backup_vm":
		return vmOp("backup", vmdir())
	case "backup_list":
		return vmOp("backup-list", vmdir())
	case "trash_list":
		return vmOp("trash-list", vmdir())
	case "export_vm":
		return func() int { return vmOp("export", vmdir(), extra["export"])() }
	case "import_vm":
		return func() int { return vmOp("import", vmdir(), extra["import"])() }
	case "backup_verify":
		return func() int { return vmOp("backup-verify", vmdir(), extra["backup-verify"])() }
	case "backup_restore":
		return func() int { return vmOp("backup-restore", vmdir(), extra["backup-restore"], ".")() }
	case "backup_prune":
		return func() int { return vmOp("backup-prune", vmdir(), extra["backup-prune"])() }
	case "trash_empty":
		return vmOp("trash-empty", vmdir())
	case "trash_restore":
		return func() int { return vmOp("trash-restore", vmdir(), extra["trash-restore"])() }
	}
	return nil
}

// dispatchSnapshotDisk: snapshot and disk ops.
func dispatchSnapshotDisk(act string) func() int {
	switch act {
	case "snapshot_create":
		return vmOp("snapshot-create", vmdir(), extra["snapshot-create"])
	case "snapshot_apply":
		return vmOp("snapshot-apply", vmdir(), extra["snapshot-apply"])
	case "snapshot_delete":
		return vmOp("snapshot-delete", vmdir(), extra["snapshot-delete"])
	case "snapshot_list":
		return vmOp("snapshot-list", vmdir())
	case "disk_compact":
		return vmOp("compact", vmdir())
	case "disk_resize":
		return vmOp("resize", vmdir(), extra["disk-resize"])
	case "delete_disk":
		return vmOp("delete-disk", vmdir(), pidPath())
	case "snapshot_tree":
		return vmOp("snapshot-tree", vmdir())
	case "snapshot_verify":
		return vmOp("snapshot-verify", vmdir())
	case "snapshot_cmd":
		return actSnapshotCmd
	}
	return nil
}

// dispatchInfo: host/report actions plus the doctor check.
func dispatchInfo(act string) func() int {
	switch act {
	case "host_info":
		return actHostInfo
	case "capabilities":
		return actCapabilities
	case "inventory_show":
		return actInventory
	case "vm_stats":
		return actStats
	case "inspect_conf":
		return actInspectConf
	case "migrate_conf":
		return actMigrateConf
	case "events_read":
		return actEvents
	case "plugin_list":
		return actPlugins
	case "doctor_vm":
		return actDoctor
	}
	return nil
}

// dispatchOps: device hotplug and op-journal actions.
func dispatchOps(act string) func() int {
	switch act {
	case "vm_hotplug_usb":
		return actHotplugUSB
	case "vm_hotplug_disk":
		return actHotplugDisk
	case "vm_hotunplug":
		return actHotunplug
	case "operation_status":
		return opsOp("status", "--id", extra["op-status"])
	case "operation_list":
		return opsOp("list")
	case "operation_cancel":
		return opsOp("cancel", "--id", extra["op-cancel"])
	case "operation_watch":
		return opsOp("watch", extra["op-watch"])
	}
	return nil
}

// dispatchViewers: console/viewer launchers.
func dispatchViewers(act string) func() int {
	switch act {
	case "spice_console":
		return actSpiceConsole
	case "vnc_viewer":
		return actVNC
	case "publicdir_open":
		return actPublicDir
	case "serial_console":
		return func() int { return actTelnet("serial") }
	case "monitor_console":
		return func() int { return actTelnet("monitor") }
	}
	return nil
}
