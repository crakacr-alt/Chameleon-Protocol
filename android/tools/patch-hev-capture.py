#!/usr/bin/env python3
from pathlib import Path

root = Path(__file__).resolve().parents[2]
hev = Path(__import__("sys").argv[1]).resolve()

def edit(path, old, new):
    p = hev / path
    text = p.read_text(encoding="utf-8")
    if old not in text:
        raise SystemExit(f"patch marker missing in {path}: {old[:80]!r}")
    p.write_text(text.replace(old, new, 1), encoding="utf-8")

# Add capture path to config.
edit("src/hev-config.c",
'''static char log_file[1024];
static char pid_file[1024];''',
'''static char log_file[1024];
static char pid_file[1024];
static char pcap_file[1024];''')

edit("src/hev-config.c",
'''        else if (0 == strcmp (key, "log-file"))
            strncpy (log_file, value, 1024 - 1);
        else if (0 == strcmp (key, "log-level"))''',
'''        else if (0 == strcmp (key, "log-file"))
            strncpy (log_file, value, 1024 - 1);
        else if (0 == strcmp (key, "pcap-file"))
            strncpy (pcap_file, value, 1024 - 1);
        else if (0 == strcmp (key, "log-level"))''')

edit("src/hev-config.c",
'''    memset (log_file, 0, sizeof (log_file));
    memset (pid_file, 0, sizeof (pid_file));''',
'''    memset (log_file, 0, sizeof (log_file));
    memset (pid_file, 0, sizeof (pid_file));
    memset (pcap_file, 0, sizeof (pcap_file));''')

edit("src/hev-config.c",
'''const char *
hev_config_get_misc_log_file (void)
{
    if (!log_file[0])
        return NULL;
    if (0 == strcmp (log_file, "null"))
        return NULL;

    return log_file;
}

int
hev_config_get_misc_log_level''',
'''const char *
hev_config_get_misc_log_file (void)
{
    if (!log_file[0])
        return NULL;
    if (0 == strcmp (log_file, "null"))
        return NULL;

    return log_file;
}

const char *
hev_config_get_misc_pcap_file (void)
{
    if (!pcap_file[0])
        return NULL;
    if (0 == strcmp (pcap_file, "null"))
        return NULL;

    return pcap_file;
}

int
hev_config_get_misc_log_level''')

edit("src/hev-config.h",
'''const char *hev_config_get_misc_log_file (void);
int hev_config_get_misc_log_level (void);''',
'''const char *hev_config_get_misc_log_file (void);
const char *hev_config_get_misc_pcap_file (void);
int hev_config_get_misc_log_level (void);''')

# PCAP writer. DLT_RAW = 101, packets begin with their IPv4/IPv6 header.
edit("src/hev-socks5-tunnel.c",
'''#include <sys/ioctl.h>''',
'''#include <sys/ioctl.h>
#include <stdio.h>
#include <stdint.h>
#include <sys/time.h>''')

edit("src/hev-socks5-tunnel.c",
'''static size_t stat_rx_bytes;

static struct netif *netif;''',
'''static size_t stat_rx_bytes;

static FILE *pcap_fp;

struct chameleon_pcap_header
{
    uint32_t magic;
    uint16_t major;
    uint16_t minor;
    int32_t zone;
    uint32_t sigfigs;
    uint32_t snaplen;
    uint32_t network;
};

struct chameleon_pcap_record
{
    uint32_t sec;
    uint32_t usec;
    uint32_t captured;
    uint32_t original;
};

static void
pcap_open (void)
{
    const char *path = hev_config_get_misc_pcap_file ();
    struct chameleon_pcap_header header = {
        0xa1b2c3d4, 2, 4, 0, 0, 65535, 101
    };

    if (!path)
        return;

    pcap_fp = fopen (path, "wb");
    if (!pcap_fp)
        return;

    fwrite (&header, sizeof (header), 1, pcap_fp);
    fflush (pcap_fp);
}

static void
pcap_close (void)
{
    if (!pcap_fp)
        return;
    fflush (pcap_fp);
    fclose (pcap_fp);
    pcap_fp = NULL;
}

static void
pcap_packet (struct pbuf *buf)
{
    struct chameleon_pcap_record record;
    struct timeval tv;
    struct pbuf *part;

    if (!pcap_fp || !buf || !buf->tot_len)
        return;

    gettimeofday (&tv, NULL);
    record.sec = (uint32_t)tv.tv_sec;
    record.usec = (uint32_t)tv.tv_usec;
    record.captured = buf->tot_len;
    record.original = buf->tot_len;

    fwrite (&record, sizeof (record), 1, pcap_fp);
    for (part = buf; part; part = part->next)
        fwrite (part->payload, part->len, 1, pcap_fp);
    fflush (pcap_fp);
}

static struct netif *netif;''')

edit("src/hev-socks5-tunnel.c",
'''static err_t
netif_output_handler (struct netif *netif, struct pbuf *p)
{
    ssize_t s;

    s = hev_tunnel_write (tun_fd, p);''',
'''static err_t
netif_output_handler (struct netif *netif, struct pbuf *p)
{
    ssize_t s;

    pcap_packet (p);
    s = hev_tunnel_write (tun_fd, p);''')

edit("src/hev-socks5-tunnel.c",
'''        if (!buf)
            continue;

        stat_tx_packets++;''',
'''        if (!buf)
            continue;

        pcap_packet (buf);
        stat_tx_packets++;''')

edit("src/hev-socks5-tunnel.c",
'''int
hev_socks5_tunnel_init (int tun_fd)
{''',
'''int
hev_socks5_tunnel_init (int extern_tun_fd)
{''')

# Open/close capture with the tunnel lifecycle.
# Insert after successful tunnel_init path through the public init/fini functions.
text = (hev / "src/hev-socks5-tunnel.c").read_text(encoding="utf-8")
marker = '''int
hev_socks5_tunnel_init (int extern_tun_fd)
'''
if marker not in text:
    raise SystemExit("hev_socks5_tunnel_init marker missing")

# Locate init body and add pcap_open after tunnel_init succeeds.
old = '''    res = tunnel_init (tun_fd);
    if (res < 0)
        goto exit;'''
new = '''    res = tunnel_init (extern_tun_fd);
    if (res < 0)
        goto exit;

    pcap_open ();'''
if old not in text:
    raise SystemExit("tunnel_init call marker missing")
text = text.replace(old, new, 1)

old = '''void
hev_socks5_tunnel_fini (void)
{'''
if old not in text:
    raise SystemExit("fini marker missing")
text = text.replace(old, '''void
hev_socks5_tunnel_fini (void)
{
    pcap_close ();
''', 1)
# The replacement added an extra opening brace if upstream body begins with it.
text = text.replace('''{
    pcap_close ();

    tunnel_fini ();''', '''    pcap_close ();

    tunnel_fini ();''', 1)
(hev / "src/hev-socks5-tunnel.c").write_text(text, encoding="utf-8")

print("hev-socks5-tunnel capture patch applied")
