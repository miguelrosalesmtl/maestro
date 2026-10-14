//go:build darwin && cgo

package term

/*
#include <libproc.h>
#include <sys/resource.h>

static long long maestro_footprint(int pid) {
	struct rusage_info_v2 ri;
	if (proc_pid_rusage(pid, RUSAGE_INFO_V2, (rusage_info_t *)&ri) != 0) return -1;
	return (long long)ri.ri_phys_footprint;
}
*/
import "C"

// footprint is a process's memory as Activity Monitor shows it: resident
// plus compressed, which is most of it on a Mac under memory pressure.
func footprint(pid int) int64 { return int64(C.maestro_footprint(C.int(pid))) }
