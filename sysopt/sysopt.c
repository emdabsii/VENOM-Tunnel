/*
 * Venom Tunnel — sysopt (Low-Level Linux Kernel & Socket Optimizer)
 * Applies BBR, FQ qdisc, high-load conntrack, TCP buffers, and multi-core RPS.
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <dirent.h>

typedef struct {
    const char *path;
    const char *value;
} SysctlEntry;

static const SysctlEntry TUNING_TABLE[] = {
    {"/proc/sys/net/ipv4/ip_forward", "1"},
    {"/proc/sys/net/ipv4/conf/all/rp_filter", "0"},
    {"/proc/sys/net/ipv4/conf/default/rp_filter", "0"},
    {"/proc/sys/net/core/default_qdisc", "fq"},
    {"/proc/sys/net/ipv4/tcp_congestion_control", "bbr"},
    {"/proc/sys/net/core/rmem_max", "67108864"},
    {"/proc/sys/net/core/wmem_max", "67108864"},
    {"/proc/sys/net/core/optmem_max", "67108864"},
    {"/proc/sys/net/ipv4/tcp_rmem", "4096 87380 67108864"},
    {"/proc/sys/net/ipv4/tcp_wmem", "4096 65536 67108864"},
    {"/proc/sys/net/core/netdev_max_backlog", "250000"},
    {"/proc/sys/net/core/somaxconn", "65535"},
    {"/proc/sys/net/ipv4/tcp_max_syn_backlog", "65535"},
    {"/proc/sys/net/ipv4/tcp_slow_start_after_idle", "0"},
    {"/proc/sys/net/ipv4/tcp_mtu_probing", "1"},
    {"/proc/sys/net/ipv4/tcp_tw_reuse", "1"},
    {"/proc/sys/net/ipv4/tcp_fin_timeout", "15"},
    {"/proc/sys/net/ipv4/tcp_fastopen", "3"},
    {"/proc/sys/net/ipv4/tcp_notsent_lowat", "16384"},
    {"/proc/sys/net/netfilter/nf_conntrack_max", "1048576"},
    {NULL, NULL}
};

static int write_proc(const char *path, const char *val) {
    FILE *f = fopen(path, "w");
    if (!f) return -1;
    int ok = (fputs(val, f) >= 0) ? 0 : -1;
    fclose(f);
    return ok;
}

static void read_proc_line(const char *path, char *buf, size_t len) {
    buf[0] = '\0';
    FILE *f = fopen(path, "r");
    if (!f) {
        snprintf(buf, len, "unavailable");
        return;
    }
    if (fgets(buf, (int)len, f)) {
        buf[strcspn(buf, "\r\n")] = '\0';
    }
    fclose(f);
}

static int apply_rps(const char *iface) {
    long cpus = sysconf(_SC_NPROCESSORS_ONLN);
    if (cpus < 1) cpus = 1;
    if (cpus > 30) cpus = 30;
    unsigned long mask = (1UL << cpus) - 1UL;
    char mask_hex[32];
    snprintf(mask_hex, sizeof(mask_hex), "%lx", mask);

    char qdir[256];
    snprintf(qdir, sizeof(qdir), "/sys/class/net/%.64s/queues", iface);
    DIR *d = opendir(qdir);
    if (!d) return 0;

    int updated = 0;
    struct dirent *ent;
    while ((ent = readdir(d)) != NULL) {
        if (strncmp(ent->d_name, "rx-", 3) == 0) {
            char rps_path[600];
            snprintf(rps_path, sizeof(rps_path), "%s/%.255s/rps_cpus", qdir, ent->d_name);
            if (write_proc(rps_path, mask_hex) == 0) updated++;
        }
    }
    closedir(d);
    write_proc("/proc/sys/net/core/rps_sock_flow_entries", "32768");
    return updated;
}

static void print_status_json(void) {
    char cc[64], qdisc[64], rmem[64], conntrack[64];
    read_proc_line("/proc/sys/net/ipv4/tcp_congestion_control", cc, sizeof(cc));
    read_proc_line("/proc/sys/net/core/default_qdisc", qdisc, sizeof(qdisc));
    read_proc_line("/proc/sys/net/core/rmem_max", rmem, sizeof(rmem));
    read_proc_line("/proc/sys/net/netfilter/nf_conntrack_max", conntrack, sizeof(conntrack));
    long cpus = sysconf(_SC_NPROCESSORS_ONLN);
    printf("{\"congestion_control\":\"%s\",\"qdisc\":\"%s\",\"rmem_max\":\"%s\",\"conntrack_max\":\"%s\",\"cpu_cores\":%ld}\n",
           cc, qdisc, rmem, conntrack, cpus);
}

int main(int argc, char **argv) {
    if (argc > 1 && strcmp(argv[1], "status") == 0) {
        print_status_json();
        return 0;
    }

    int applied = 0, skipped = 0;
    for (int i = 0; TUNING_TABLE[i].path != NULL; i++) {
        if (write_proc(TUNING_TABLE[i].path, TUNING_TABLE[i].value) == 0) {
            applied++;
        } else {
            skipped++;
        }
    }

    const char *iface = (argc > 2 && strcmp(argv[1], "apply") == 0) ? argv[2] : "eth0";
    int rps_queues = apply_rps(iface);

    printf("{\"status\":\"ok\",\"applied\":%d,\"skipped\":%d,\"rps_queues\":%d,\"iface\":\"%s\"}\n",
           applied, skipped, rps_queues, iface);
    return 0;
}
