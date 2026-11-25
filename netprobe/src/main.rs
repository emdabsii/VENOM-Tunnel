//! Venom Tunnel — netprobe (Rust Network Diagnostics & Telemetry Engine)
//! Zero external crate dependencies for rock-solid offline/cross-arch builds.

use std::env;
use std::fs;
use std::net::{SocketAddr, TcpStream, ToSocketAddrs};
use std::thread;
use std::time::{Duration, Instant};

fn probe_tcp(host: &str, port: u16, count: usize, timeout_ms: u64) {
    let target = format!("{}:{}", host, port);
    let addr: SocketAddr = match target.to_socket_addrs().ok().and_then(|mut it| it.next()) {
        Some(a) => a,
        None => {
            println!("{{\"error\":\"dns_resolve_failed\",\"target\":\"{}\"}}", target);
            return;
        }
    };

    let timeout = Duration::from_millis(timeout_ms);
    let mut ok = 0usize;
    let mut rtts: Vec<f64> = Vec::with_capacity(count);

    for i in 0..count {
        let start = Instant::now();
        if TcpStream::connect_timeout(&addr, timeout).is_ok() {
            ok += 1;
            rtts.push(start.elapsed().as_secs_f64() * 1000.0);
        }
        if i + 1 < count {
            thread::sleep(Duration::from_millis(120));
        }
    }

    let loss = if count > 0 {
        ((count - ok) as f64 / count as f64) * 100.0
    } else {
        100.0
    };
    let (min_ms, avg_ms, max_ms) = if rtts.is_empty() {
        (0.0, 0.0, 0.0)
    } else {
        let min = rtts.iter().cloned().fold(f64::INFINITY, f64::min);
        let max = rtts.iter().cloned().fold(0.0, f64::max);
        let avg = rtts.iter().sum::<f64>() / rtts.len() as f64;
        (min, avg, max)
    };

    println!(
        "{{\"host\":\"{}\",\"port\":{},\"sent\":{},\"received\":{},\"loss_pct\":{:.1},\"min_ms\":{:.2},\"avg_ms\":{:.2},\"max_ms\":{:.2}}}",
        host, port, count, ok, loss, min_ms, avg_ms, max_ms
    );
}

fn check_international() {
    let targets = [
        ("cloudflare", "1.1.1.1:443"),
        ("google", "8.8.8.8:443"),
        ("github", "140.82.121.3:443"),
    ];
    let timeout = Duration::from_millis(2500);
    let mut open_count = 0usize;
    let mut items = Vec::new();

    for (name, ep) in targets {
        let start = Instant::now();
        let addr: SocketAddr = ep.parse().unwrap();
        let reachable = TcpStream::connect_timeout(&addr, timeout).is_ok();
        let ms = if reachable {
            open_count += 1;
            start.elapsed().as_secs_f64() * 1000.0
        } else {
            0.0
        };
        items.push(format!(
            "{{\"name\":\"{}\",\"endpoint\":\"{}\",\"open\":{},\"rtt_ms\":{:.1}}}",
            name, ep, reachable, ms
        ));
    }

    println!(
        "{{\"international_open\":{},\"reachable_count\":{},\"checks\":[{}]}}",
        open_count > 0,
        open_count,
        items.join(",")
    );
}

fn read_cpu_pct() -> f64 {
    fn sample() -> (u64, u64) {
        let content = fs::read_to_string("/proc/stat").unwrap_or_default();
        if let Some(line) = content.lines().next() {
            let nums: Vec<u64> = line
                .split_whitespace()
                .skip(1)
                .filter_map(|v| v.parse().ok())
                .collect();
            if nums.len() >= 4 {
                let idle = nums[3] + nums.get(4).copied().unwrap_or(0);
                let total: u64 = nums.iter().sum();
                return (idle, total);
            }
        }
        (0, 0)
    }
    let (i1, t1) = sample();
    thread::sleep(Duration::from_millis(180));
    let (i2, t2) = sample();
    let dt = t2.saturating_sub(t1);
    let di = i2.saturating_sub(i1);
    if dt == 0 { 0.0 } else { (1.0 - (di as f64 / dt as f64)) * 100.0 }
}

fn read_mem_pct() -> (f64, u64, u64) {
    let content = fs::read_to_string("/proc/meminfo").unwrap_or_default();
    let mut total = 0u64;
    let mut avail = 0u64;
    for line in content.lines() {
        if line.starts_with("MemTotal:") {
            total = line.split_whitespace().nth(1).and_then(|v| v.parse().ok()).unwrap_or(0);
        } else if line.starts_with("MemAvailable:") {
            avail = line.split_whitespace().nth(1).and_then(|v| v.parse().ok()).unwrap_or(0);
        }
    }
    let used = total.saturating_sub(avail);
    let pct = if total > 0 { (used as f64 / total as f64) * 100.0 } else { 0.0 };
    (pct, used / 1024, total / 1024)
}

fn read_net_bytes() -> (u64, u64) {
    let content = fs::read_to_string("/proc/net/dev").unwrap_or_default();
    let mut rx = 0u64;
    let mut tx = 0u64;
    for line in content.lines().skip(2) {
        let parts: Vec<&str> = line.split(':').collect();
        if parts.len() != 2 { continue; }
        let iface = parts[0].trim();
        if iface == "lo" || iface.starts_with("docker") { continue; }
        let nums: Vec<u64> = parts[1].split_whitespace().filter_map(|v| v.parse().ok()).collect();
        if nums.len() >= 9 {
            rx += nums[0];
            tx += nums[8];
        }
    }
    (rx, tx)
}

fn telemetry() {
    let (rx1, tx1) = read_net_bytes();
    let cpu = read_cpu_pct(); // sleeps 180ms
    let (rx2, tx2) = read_net_bytes();
    let (ram_pct, ram_used_mb, ram_total_mb) = read_mem_pct();
    let rx_mbps = ((rx2.saturating_sub(rx1)) as f64 * 8.0) / (0.180 * 1_000_000.0);
    let tx_mbps = ((tx2.saturating_sub(tx1)) as f64 * 8.0) / (0.180 * 1_000_000.0);
    let uptime = fs::read_to_string("/proc/uptime")
        .ok()
        .and_then(|s| s.split_whitespace().next().and_then(|v| v.parse::<f64>().ok()))
        .unwrap_or(0.0);

    println!(
        "{{\"cpu_pct\":{:.1},\"ram_pct\":{:.1},\"ram_used_mb\":{},\"ram_total_mb\":{},\"rx_mbps\":{:.2},\"tx_mbps\":{:.2},\"rx_total_gb\":{:.2},\"tx_total_gb\":{:.2},\"uptime_sec\":{:.0}}}",
        cpu, ram_pct, ram_used_mb, ram_total_mb, rx_mbps, tx_mbps,
        rx2 as f64 / 1_073_741_824.0, tx2 as f64 / 1_073_741_824.0, uptime
    );
}

fn main() {
    let args: Vec<String> = env::args().collect();
    if args.len() < 2 {
        eprintln!("Usage: venom-netprobe <telemetry|intl|tcp HOST PORT [COUNT]>");
        std::process::exit(1);
    }
    match args[1].as_str() {
        "telemetry" => telemetry(),
        "intl" => check_international(),
        "tcp" => {
            if args.len() < 4 {
                eprintln!("Usage: venom-netprobe tcp HOST PORT [COUNT]");
                std::process::exit(1);
            }
            let host = &args[2];
            let port: u16 = args[3].parse().unwrap_or(443);
            let count: usize = args.get(4).and_then(|v| v.parse().ok()).unwrap_or(10);
            probe_tcp(host, port, count, 2000);
        }
        _ => {
            eprintln!("Unknown subcommand");
            std::process::exit(1);
        }
    }
}
