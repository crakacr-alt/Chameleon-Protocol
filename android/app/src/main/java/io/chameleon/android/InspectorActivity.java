package io.chameleon.android;

import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.net.ConnectivityManager;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.graphics.Typeface;
import android.view.Gravity;
import android.widget.Button;
import android.widget.EditText;
import android.widget.LinearLayout;
import android.widget.ListView;
import android.widget.ArrayAdapter;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.RandomAccessFile;
import java.net.InetAddress;
import java.net.InetSocketAddress;
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.text.SimpleDateFormat;
import java.util.ArrayList;
import java.util.Collections;
import java.util.Comparator;
import java.util.Date;
import java.util.HashMap;
import java.util.List;
import java.util.Locale;
import java.util.Map;

import hev.htproxy.TProxyService;

/**
 * Visible, user-started traffic inspector for the local Android device.
 *
 * Capture comes from the VpnService TUN and is written as standard DLT_RAW
 * PCAP by the pinned tun2socks build. This screen only reads local capture
 * data and never enables capture silently.
 */
public final class InspectorActivity extends Activity {
    private static final int EXPORT_PCAP = 3101;
    private static final int MAX_VISIBLE_FLOWS = 80;
    private static final int MAX_PACKET_SIZE = 1024 * 1024;

    private final Handler handler = new Handler(Looper.getMainLooper());
    private final Map<String, Flow> flows = new HashMap<>();
    private final List<Packet> packets = new ArrayList<>();

    private TextView statusText;
    private TextView statsText;
    private TextView flowText;
    private TextView headingText;
    private EditText filterText;
    private ListView packetList;
    private ArrayAdapter<String> packetAdapter;
    private final List<Packet> visiblePackets = new ArrayList<>();
    private boolean showPackets;
    private boolean paused;
    private long packetNumber;

    private long parsedOffset;
    private boolean pcapLittleEndian = true;

    private final Runnable refresh = new Runnable() {
        @Override
        public void run() {
            refreshInspector();
            handler.postDelayed(this, 1000);
        }
    };

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        setContentView(buildUi());
    }

    @Override
    protected void onResume() {
        super.onResume();
        handler.removeCallbacks(refresh);
        handler.post(refresh);
    }

    @Override
    protected void onPause() {
        handler.removeCallbacks(refresh);
        super.onPause();
    }

    private ScrollView buildUi() {
        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        scroll.setBackgroundColor(Color.rgb(10, 17, 30));

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setPadding(dp(18), dp(20), dp(18), dp(28));
        scroll.addView(root, new ScrollView.LayoutParams(-1, -2));

        TextView title = text("Chameleon Inspector", 24, Color.WHITE);
        title.setGravity(Gravity.START);
        root.addView(title);

        TextView subtitle = text(
                "Системный анализатор трафика • TUN → PCAP → Wireshark",
                13, Color.rgb(148, 163, 184));
        root.addView(subtitle, top(dp(4)));

        statusText = text("Ожидание…", 15, Color.WHITE);
        root.addView(statusText, top(dp(18)));

        statsText = text("Пакеты: —", 13, Color.rgb(203, 213, 225));
        root.addView(statsText, top(dp(6)));

        filterText = new EditText(this);
        filterText.setHint("Фильтр: приложение, домен, IP, TCP/UDP");
        filterText.setHintTextColor(Color.rgb(100, 116, 139));
        filterText.setTextColor(Color.WHITE);
        filterText.setSingleLine(true);
        root.addView(filterText, top(dp(14)));

        Button view = button("Показать отдельные пакеты");
        view.setOnClickListener(v -> {
            showPackets = !showPackets;
            view.setText(showPackets ? "Показать потоки" : "Показать отдельные пакеты");
            refreshInspector();
        });
        root.addView(view, top(dp(8)));
        Button pause = button("Пауза просмотра");
        pause.setOnClickListener(v -> {
            paused = !paused;
            pause.setText(paused ? "Продолжить просмотр" : "Пауза просмотра");
            if (!paused) refreshInspector();
        });
        root.addView(pause, top(dp(8)));

        Button export = button("Экспорт PCAP для Wireshark");
        export.setOnClickListener(v -> exportPcap());
        root.addView(export, top(dp(12)));

        Button clear = button("Очистить захват");
        clear.setOnClickListener(v -> clearCapture());
        root.addView(clear, top(dp(8)));

        TextView caNote = text(
                "Захват содержит IP-пакеты, DNS и доступные имена TLS. "
                        + "Содержимое HTTPS зашифровано: эта версия не выполняет TLS-расшифровку. "
                        + "Для подробного анализа откройте экспорт PCAP в Wireshark.",
                12, Color.rgb(148, 163, 184));
        root.addView(caNote, top(dp(12)));

        headingText = text("АКТИВНЫЕ / НЕДАВНИЕ ПОТОКИ", 12, Color.rgb(148, 163, 184));
        root.addView(headingText, top(dp(22)));

        flowText = text("Пока нет пакетов.", 13, Color.rgb(226, 232, 240));
        flowText.setTextIsSelectable(true);
        root.addView(flowText, top(dp(8)));

        packetList = new ListView(this);
        packetList.setBackgroundColor(Color.rgb(18, 28, 44));
        packetAdapter = new ArrayAdapter<String>(this, android.R.layout.simple_list_item_1, new ArrayList<>()) {
            @Override public android.view.View getView(int position, android.view.View convertView,
                    android.view.ViewGroup parent) {
                TextView row = (TextView) super.getView(position, convertView, parent);
                row.setTextColor(Color.rgb(226, 232, 240));
                row.setTextSize(12);
                return row;
            }
        };
        packetList.setAdapter(packetAdapter);
        packetList.setOnItemClickListener((parent, item, position, id) -> {
            if (position < visiblePackets.size()) showPacket(visiblePackets.get(position));
        });
        root.addView(packetList, new LinearLayout.LayoutParams(-1, dp(360)));
        packetList.setVisibility(android.view.View.GONE);

        return scroll;
    }

    private void refreshInspector() {
        // Pausing freezes the display; the native writer continues capturing.
        if (paused) return;
        boolean active = ChameleonVpnService.running()
                && "inspector".equals(AppFiles.runtimeMode(this));
        File capture = AppFiles.captureFile(this);

        statusText.setText(active
                ? "● Inspector активен • запись PCAP идёт"
                : "○ Inspector не активен");

        try {
            long[] stats = TProxyService.TProxyGetStats();
            if (stats != null && stats.length >= 4) {
                statsText.setText(
                        "TX: " + stats[0] + " пак. / " + humanBytes(stats[1])
                                + "   RX: " + stats[2] + " пак. / " + humanBytes(stats[3])
                                + "   PCAP: " + humanBytes(capture.isFile() ? capture.length() : 0));
            }
        } catch (Throwable error) {
            statsText.setText("Статистика TUN пока недоступна");
        }

        if (capture.isFile()) {
            try {
                parseNewPackets(capture);
            } catch (Exception error) {
                statusText.setText("Ошибка чтения PCAP: " + error.getMessage());
            }
        }
        renderFlows();
        renderPackets();
    }

    private void parseNewPackets(File file) throws Exception {
        try (RandomAccessFile raf = new RandomAccessFile(file, "r")) {
            long length = raf.length();
            if (length < 24) return;

            if (parsedOffset == 0 || parsedOffset > length) {
                flows.clear();
                packets.clear();
                packetNumber = 0;
                parsedOffset = 24;
                raf.seek(0);
                byte[] global = new byte[24];
                raf.readFully(global);
                int magicLe = ByteBuffer.wrap(global, 0, 4)
                        .order(ByteOrder.LITTLE_ENDIAN).getInt();
                int magicBe = ByteBuffer.wrap(global, 0, 4)
                        .order(ByteOrder.BIG_ENDIAN).getInt();
                if (magicLe == 0xa1b2c3d4) {
                    pcapLittleEndian = true;
                } else if (magicBe == 0xa1b2c3d4) {
                    pcapLittleEndian = false;
                } else {
                    throw new IllegalStateException("неизвестный PCAP header");
                }
                if (ByteBuffer.wrap(global).order(pcapLittleEndian
                        ? ByteOrder.LITTLE_ENDIAN : ByteOrder.BIG_ENDIAN).getInt(20) != 101)
                    throw new IllegalStateException("ожидается PCAP с RAW IP-пакетами");
            }

            raf.seek(parsedOffset);
            ByteOrder order = pcapLittleEndian ? ByteOrder.LITTLE_ENDIAN : ByteOrder.BIG_ENDIAN;
            while (raf.getFilePointer() + 16 <= length) {
                byte[] header = new byte[16];
                raf.readFully(header);
                ByteBuffer h = ByteBuffer.wrap(header).order(order);
                long sec = Integer.toUnsignedLong(h.getInt());
                long usec = Integer.toUnsignedLong(h.getInt());
                int incl = h.getInt();
                h.getInt(); // original length

                if (incl <= 0 || incl > MAX_PACKET_SIZE) {
                    throw new IllegalStateException("некорректная длина пакета " + incl);
                }
                if (raf.getFilePointer() + incl > length) {
                    raf.seek(raf.getFilePointer() - 16);
                    break;
                }
                byte[] packet = new byte[incl];
                raf.readFully(packet);
                parsePacket(packet, sec * 1000L + usec / 1000L);
            }
            parsedOffset = raf.getFilePointer();
        }
    }

    private void parsePacket(byte[] packet, long when) {
        if (packet.length < 20) return;
        int version = (packet[0] >>> 4) & 0x0f;

        Packet entry = new Packet();
        entry.number = ++packetNumber;
        entry.when = when;
        entry.length = packet.length;
        entry.data = java.util.Arrays.copyOf(packet, Math.min(packet.length, 2048));
        entry.summary = "IPv" + version + " • " + packet.length + " B";
        try {
            int addressOffset = version == 6 ? 8 : 12;
            int addressLength = version == 6 ? 16 : 4;
            String src = InetAddress.getByAddress(java.util.Arrays.copyOfRange(packet,
                    addressOffset, addressOffset + addressLength)).getHostAddress();
            String dst = InetAddress.getByAddress(java.util.Arrays.copyOfRange(packet,
                    addressOffset + addressLength, addressOffset + 2 * addressLength)).getHostAddress();
            int[] transport = version == 6 ? ipv6Transport(packet)
                    : new int[]{(packet[0] & 15) * 4, packet[9] & 255};
            int protocol = transport[1];
            int offset = transport[0];
            String proto = protocol == 6 ? "TCP" : protocol == 17 ? "UDP"
                    : protocol == 1 || protocol == 58 ? "ICMP" : "IP " + protocol;
            if ((protocol == 6 || protocol == 17) && offset + 4 <= packet.length) {
                src += ":" + u16(packet, offset);
                dst += ":" + u16(packet, offset + 2);
            }
            entry.summary = proto + " " + src + " → " + dst + " • " + packet.length + " B";
        } catch (Exception ignored) { }
        packets.add(entry);
        if (packets.size() > 200) packets.remove(0);

        try {
            if (version == 4) {
                parseIpv4(packet, when);
            } else if (version == 6) {
                parseIpv6(packet, when);
            }
        } catch (Exception ignored) {
        }
    }

    private void parseIpv4(byte[] p, long when) throws Exception {
        int ihl = (p[0] & 0x0f) * 4;
        if (ihl < 20 || p.length < ihl + 4) return;
        if ((u16(p, 6) & 0x1fff) != 0) return; // noninitial IP fragment
        int protocol = p[9] & 0xff;
        byte[] srcRaw = new byte[4];
        byte[] dstRaw = new byte[4];
        System.arraycopy(p, 12, srcRaw, 0, 4);
        System.arraycopy(p, 16, dstRaw, 0, 4);
        handleTransport(p, ihl, protocol,
                InetAddress.getByAddress(srcRaw), InetAddress.getByAddress(dstRaw), when);
    }

    private void parseIpv6(byte[] p, long when) throws Exception {
        if (p.length < 44) return;
        int[] transport = ipv6Transport(p);
        byte[] srcRaw = new byte[16];
        byte[] dstRaw = new byte[16];
        System.arraycopy(p, 8, srcRaw, 0, 16);
        System.arraycopy(p, 24, dstRaw, 0, 16);
        handleTransport(p, transport[0], transport[1],
                InetAddress.getByAddress(srcRaw), InetAddress.getByAddress(dstRaw), when);
    }

    private static int[] ipv6Transport(byte[] p) {
        int offset = 40;
        int protocol = p[6] & 255;
        for (int count = 0; count < 8; count++) {
            if (protocol != 0 && protocol != 43 && protocol != 60 && protocol != 51 && protocol != 44)
                return new int[]{offset, protocol};
            if (offset + 8 > p.length) return new int[]{p.length, -1};
            if (protocol == 44 && (u16(p, offset + 2) & 0xfff8) != 0)
                return new int[]{p.length, -1};
            int size = protocol == 44 ? 8 : protocol == 51
                    ? ((p[offset + 1] & 255) + 2) * 4 : ((p[offset + 1] & 255) + 1) * 8;
            protocol = p[offset] & 255;
            offset += size;
            if (offset > p.length) return new int[]{p.length, -1};
        }
        return new int[]{p.length, -1};
    }

    private void handleTransport(
            byte[] p, int offset, int protocol,
            InetAddress src, InetAddress dst, long when) {
        if (protocol != 6 && protocol != 17) return;
        if (p.length < offset + (protocol == 6 ? 20 : 8)) return;

        int srcPort = u16(p, offset);
        int dstPort = u16(p, offset + 2);
        boolean outgoing = isLocalVpnAddress(src);
        InetAddress local = outgoing ? src : dst;
        InetAddress remote = outgoing ? dst : src;
        int localPort = outgoing ? srcPort : dstPort;
        int remotePort = outgoing ? dstPort : srcPort;

        String proto = protocol == 6 ? "TCP" : "UDP";
        String key = proto + "|" + local.getHostAddress() + ":" + localPort
                + "|" + remote.getHostAddress() + ":" + remotePort;

        Flow flow = flows.get(key);
        if (flow == null) {
            if (flows.size() >= 2048) {
                Flow oldest = Collections.min(flows.values(), Comparator.comparingLong(f -> f.lastSeen));
                flows.values().remove(oldest);
            }
            flow = new Flow();
            flow.protocol = proto;
            flow.local = local.getHostAddress() + ":" + localPort;
            flow.remote = remote.getHostAddress() + ":" + remotePort;
            flow.firstSeen = when;
            resolveOwner(flow, protocol, local, localPort, remote, remotePort);
            flows.put(key, flow);
        }
        flow.lastSeen = when;
        flow.packets++;
        flow.bytes += p.length;

        int payloadOffset;
        if (protocol == 6) {
            int tcpHeader = ((p[offset + 12] >>> 4) & 0x0f) * 4;
            if (tcpHeader < 20 || offset + tcpHeader > p.length) return;
            payloadOffset = offset + tcpHeader;
        } else {
            payloadOffset = offset + 8;
        }
        if (payloadOffset >= p.length) return;

        if (remotePort == 53 || localPort == 53) {
            String q = parseDnsName(p, payloadOffset + (protocol == 6 ? 2 : 0));
            if (q != null && !q.isEmpty()) flow.detail = "DNS " + q;
        } else if (protocol == 6) {
            String sni = parseTlsSni(p, payloadOffset);
            if (sni != null) {
                flow.detail = "TLS SNI " + sni;
            } else {
                String http = detectHttp(p, payloadOffset);
                if (http != null) flow.detail = http;
            }
        }
    }

    private void resolveOwner(
            Flow flow, int protocol,
            InetAddress local, int localPort,
            InetAddress remote, int remotePort) {
        try {
            ConnectivityManager cm = getSystemService(ConnectivityManager.class);
            int uid = cm.getConnectionOwnerUid(
                    protocol,
                    new InetSocketAddress(local, localPort),
                    new InetSocketAddress(remote, remotePort));
            flow.uid = uid;
            if (uid >= 0) {
                PackageManager pm = getPackageManager();
                String[] packages = pm.getPackagesForUid(uid);
                if (packages != null && packages.length > 0) {
                    flow.app = packages[0];
                    try {
                        flow.app = pm.getApplicationLabel(
                                pm.getApplicationInfo(packages[0], 0)).toString()
                                + " (" + packages[0] + ")";
                    } catch (Exception ignored) {
                    }
                }
            }
        } catch (Throwable ignored) {
        }
        if (flow.app == null) flow.app = "UID/приложение не определено";
    }

    private String parseDnsName(byte[] p, int off) {
        if (p.length < off + 13) return null;
        int qd = u16(p, off + 4);
        if (qd <= 0) return null;
        int pos = off + 12;
        StringBuilder name = new StringBuilder();
        int labels = 0;
        while (pos < p.length && labels++ < 64) {
            int len = p[pos++] & 0xff;
            if (len == 0) break;
            if ((len & 0xc0) != 0 || len > 63 || pos + len > p.length) return null;
            if (name.length() > 0) name.append('.');
            for (int i = 0; i < len; i++) {
                int ch = p[pos++] & 0xff;
                if (ch < 32 || ch > 126) return null;
                name.append((char) ch);
            }
        }
        return name.length() == 0 ? null : name.toString();
    }

    private String parseTlsSni(byte[] p, int off) {
        try {
            if (p.length < off + 5 || (p[off] & 0xff) != 0x16) return null;
            int recordLen = u16(p, off + 3);
            int end = Math.min(p.length, off + 5 + recordLen);
            int pos = off + 5;
            if (pos + 4 > end || (p[pos] & 0xff) != 0x01) return null;
            pos += 4;
            if (pos + 34 > end) return null;
            pos += 34;
            int sessionLen = p[pos++] & 0xff;
            pos += sessionLen;
            if (pos + 2 > end) return null;
            int cipherLen = u16(p, pos);
            pos += 2 + cipherLen;
            if (pos >= end) return null;
            int compLen = p[pos++] & 0xff;
            pos += compLen;
            if (pos + 2 > end) return null;
            int extLen = u16(p, pos);
            pos += 2;
            int extEnd = Math.min(end, pos + extLen);

            while (pos + 4 <= extEnd) {
                int type = u16(p, pos);
                int len = u16(p, pos + 2);
                pos += 4;
                if (pos + len > extEnd) return null;
                if (type == 0 && len >= 5) {
                    int listEnd = pos + len;
                    int q = pos + 2;
                    while (q + 3 <= listEnd) {
                        int nameType = p[q++] & 0xff;
                        int nameLen = u16(p, q);
                        q += 2;
                        if (q + nameLen > listEnd) return null;
                        if (nameType == 0) {
                            return new String(p, q, nameLen, java.nio.charset.StandardCharsets.US_ASCII);
                        }
                        q += nameLen;
                    }
                }
                pos += len;
            }
        } catch (Exception ignored) {
        }
        return null;
    }

    private String detectHttp(byte[] p, int off) {
        int len = Math.min(16, p.length - off);
        if (len <= 0) return null;
        String prefix = new String(p, off, len, java.nio.charset.StandardCharsets.US_ASCII);
        String[] methods = {"GET ", "POST ", "PUT ", "HEAD ", "DELETE ", "PATCH ", "OPTIONS "};
        for (String method : methods) {
            if (prefix.startsWith(method)) return "HTTP " + method.trim();
        }
        return null;
    }

    private void renderFlows() {
        flowText.setVisibility(showPackets ? android.view.View.GONE : android.view.View.VISIBLE);
        String filter = filterText == null ? "" : filterText.getText().toString().trim().toLowerCase(Locale.ROOT);
        List<Flow> list = new ArrayList<>(flows.values());
        Collections.sort(list, Comparator.comparingLong((Flow f) -> f.lastSeen).reversed());

        StringBuilder out = new StringBuilder();
        int shown = 0;
        for (Flow flow : list) {
            String line = flow.app + "\n"
                    + flow.protocol + "  " + flow.remote
                    + (flow.detail == null ? "" : "  •  " + flow.detail)
                    + "\n" + humanBytes(flow.bytes) + " • " + flow.packets + " пак. • UID " + flow.uid + "\n\n";
            if (!filter.isEmpty() && !line.toLowerCase(Locale.ROOT).contains(filter)) continue;
            out.append(line);
            if (++shown >= MAX_VISIBLE_FLOWS) break;
        }
        flowText.setText(out.length() == 0 ? "Пока нет подходящих потоков." : out.toString());
    }

    private void renderPackets() {
        headingText.setText(showPackets ? "ПОСЛЕДНИЕ 200 ПАКЕТОВ • НАЖМИТЕ ДЛЯ HEX/ASCII"
                : "АКТИВНЫЕ / НЕДАВНИЕ ПОТОКИ");
        packetList.setVisibility(showPackets ? android.view.View.VISIBLE : android.view.View.GONE);
        if (!showPackets) return;
        String filter = filterText.getText().toString().trim().toLowerCase(Locale.ROOT);
        visiblePackets.clear();
        packetAdapter.clear();
        for (int i = packets.size() - 1; i >= 0; i--) {
            Packet packet = packets.get(i);
            if (!filter.isEmpty() && !packet.summary.toLowerCase(Locale.ROOT).contains(filter)) continue;
            visiblePackets.add(packet);
            packetAdapter.add("#" + packet.number + " " + packet.summary);
        }
    }

    private void showPacket(Packet packet) {
        StringBuilder out = new StringBuilder(packet.summary);
        out.append("\n").append(new SimpleDateFormat("HH:mm:ss.SSS", Locale.US).format(new Date(packet.when)));
        out.append("\n\nOffset  HEX                      ASCII\n");
        for (int offset = 0; offset < packet.data.length; offset += 8) {
            out.append(String.format(Locale.US, "%04x    ", offset));
            for (int i = 0; i < 8; i++) out.append(offset + i < packet.data.length
                    ? String.format(Locale.US, "%02x ", packet.data[offset + i] & 255) : "   ");
            out.append(" ");
            for (int i = offset; i < Math.min(offset + 8, packet.data.length); i++) {
                int value = packet.data[i] & 255;
                out.append(value >= 32 && value < 127 ? (char) value : '.');
            }
            out.append('\n');
        }
        if (packet.length > packet.data.length) out.append("\nПоказаны первые 2048 байт; полный пакет есть в PCAP.");
        TextView detail = text(out.toString(), 11, Color.WHITE);
        detail.setTypeface(Typeface.MONOSPACE);
        detail.setTextIsSelectable(true);
        detail.setPadding(dp(12), dp(12), dp(12), dp(12));
        ScrollView scroll = new ScrollView(this);
        scroll.setBackgroundColor(Color.rgb(10, 17, 30));
        scroll.addView(detail);
        new AlertDialog.Builder(this).setTitle("Пакет #" + packet.number)
                .setView(scroll).setPositiveButton("Закрыть", null).show();
    }

    private void exportPcap() {
        File capture = AppFiles.captureFile(this);
        if (!capture.isFile() || capture.length() <= 24) {
            Toast.makeText(this, "В захвате пока нет пакетов", Toast.LENGTH_SHORT).show();
            return;
        }
        String stamp = new SimpleDateFormat("yyyyMMdd-HHmmss", Locale.US).format(new Date());
        Intent intent = new Intent(Intent.ACTION_CREATE_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType("application/vnd.tcpdump.pcap");
        intent.putExtra(Intent.EXTRA_TITLE, "chameleon-" + stamp + ".pcap");
        startActivityForResult(intent, EXPORT_PCAP);
    }

    private void clearCapture() {
        if (ChameleonVpnService.running() && "inspector".equals(AppFiles.runtimeMode(this))) {
            new AlertDialog.Builder(this)
                    .setTitle("Inspector активен")
                    .setMessage("Сначала остановите Inspector, затем очистите захват.")
                    .setPositiveButton("OK", null)
                    .show();
            return;
        }
        File capture = AppFiles.captureFile(this);
        if (capture.exists() && !capture.delete()) {
            Toast.makeText(this, "Не удалось удалить PCAP", Toast.LENGTH_LONG).show();
            return;
        }
        flows.clear();
        packets.clear();
        packetNumber = 0;
        parsedOffset = 0;
        refreshInspector();
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode != EXPORT_PCAP || resultCode != RESULT_OK || data == null) return;
        Uri uri = data.getData();
        if (uri == null) return;

        try (FileInputStream in = new FileInputStream(AppFiles.captureFile(this));
             java.io.OutputStream out = getContentResolver().openOutputStream(uri)) {
            if (out == null) throw new IllegalStateException("не удалось открыть файл назначения");
            byte[] buffer = new byte[64 * 1024];
            for (int n; (n = in.read(buffer)) >= 0; ) out.write(buffer, 0, n);
            out.flush();
            Toast.makeText(this, "PCAP сохранён", Toast.LENGTH_SHORT).show();
        } catch (Exception error) {
            Toast.makeText(this, "Ошибка экспорта: " + error.getMessage(), Toast.LENGTH_LONG).show();
        }
    }

    private boolean isLocalVpnAddress(InetAddress address) {
        String value = address.getHostAddress();
        return "198.18.0.1".equals(value)
                || (value != null && value.toLowerCase(Locale.ROOT).startsWith("fd00:1:fd00:1:"));
    }

    private static int u16(byte[] p, int off) {
        return ((p[off] & 0xff) << 8) | (p[off + 1] & 0xff);
    }

    private static String humanBytes(long bytes) {
        if (bytes < 1024) return bytes + " B";
        double kb = bytes / 1024.0;
        if (kb < 1024) return String.format(Locale.US, "%.1f KB", kb);
        double mb = kb / 1024.0;
        if (mb < 1024) return String.format(Locale.US, "%.1f MB", mb);
        return String.format(Locale.US, "%.2f GB", mb / 1024.0);
    }

    private TextView text(String value, int sp, int color) {
        TextView view = new TextView(this);
        view.setText(value);
        view.setTextSize(sp);
        view.setTextColor(color);
        return view;
    }

    private Button button(String value) {
        Button button = new Button(this);
        button.setText(value);
        button.setAllCaps(false);
        return button;
    }

    private LinearLayout.LayoutParams top(int margin) {
        LinearLayout.LayoutParams p = new LinearLayout.LayoutParams(-1, -2);
        p.topMargin = margin;
        return p;
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }

    private static final class Flow {
        String protocol;
        String local;
        String remote;
        String app;
        String detail;
        int uid = -1;
        long packets;
        long bytes;
        long firstSeen;
        long lastSeen;
    }

    private static final class Packet {
        long number;
        long when;
        int length;
        byte[] data;
        String summary;
    }
}
