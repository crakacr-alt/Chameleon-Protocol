package io.chameleon.android;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.net.VpnService;
import android.os.Build;
import android.os.ParcelFileDescriptor;

import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;

import hev.htproxy.TProxyService;
import mobile.Mobile;

public final class ChameleonVpnService extends VpnService {
    static final String ACTION_START = "io.chameleon.android.VPN_START";
    static final String ACTION_STOP = "io.chameleon.android.VPN_STOP";
    static final String RUNTIME_OWNER = "vpn";

    private static final String CHANNEL_ID = "chameleon_vpn";
    private static final int NOTIFICATION_ID = 1002;

    private static volatile boolean active;
    private static volatile boolean starting;

    private final Object lock = new Object();
    private ParcelFileDescriptor tun;
    private volatile boolean stopping;

    static boolean running() { return active; }
    static boolean starting() { return starting; }

    static void requestStop(Context context) {
        Intent stop = new Intent(context, ChameleonVpnService.class);
        stop.setAction(ACTION_STOP);
        context.startService(stop);
    }

    @Override
    public void onCreate() {
        super.onCreate();
        createChannel();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        String action = intent == null ? ACTION_START : intent.getAction();

        if (ACTION_STOP.equals(action)) {
            shutdown(true);
            return START_NOT_STICKY;
        }

        if (active || starting || TProxyService.TProxyIsRunning()) {
            notifyState(active ? "VPN уже подключён" : "VPN подключается…");
            return START_STICKY;
        }

        startForeground(NOTIFICATION_ID, notification("Подключение VPN…"));
        stopping = false;
        starting = true;
        active = false;
        ChameleonWidget.updateAll(this);
        ChameleonTile.requestRefresh(this);
        new Thread(this::startTunnel, "chameleon-vpn").start();
        return START_STICKY;
    }

    private void startTunnel() {
        try {
            stopService(new Intent(this, ChameleonService.class));
            Mobile.stopOwned(ChameleonService.RUNTIME_OWNER);

            AppFiles.setRuntimeMode(this, "vpn");
            AppFiles.setCoreMode(this, "proxy");

            notifyState("Проверка доступного транспорта…");
            String vpnConfig = Mobile.prepareVPNConfig(AppFiles.readConfig(this));
            if (vpnConfig.startsWith("ERROR:")) {
                throw new IllegalStateException(vpnConfig.substring("ERROR:".length()).trim());
            }

            String error = Mobile.startOwned(vpnConfig, RUNTIME_OWNER);
            if (error != null && !error.isEmpty()) {
                throw new IllegalStateException(error);
            }
            if (!RUNTIME_OWNER.equals(Mobile.owner()) || !Mobile.listenerReady()) {
                throw new IllegalStateException("SOCKS5 127.0.0.1:1080 не запустился");
            }

            ParcelFileDescriptor established = new Builder()
                    .setSession("Chameleon VPN")
                    .setMtu(1400)
                    .addAddress("198.18.0.1", 32)
                    .addRoute("0.0.0.0", 0)
                    .addAddress("fd00:1:fd00:1::1", 128)
                    .addRoute("::", 0)
                    .addDnsServer("1.1.1.1")
                    .addDnsServer("8.8.8.8")
                    .addDisallowedApplication(getPackageName())
                    .setBlocking(true)
                    .establish();

            if (established == null) {
                throw new IllegalStateException("Android не создал VPN-интерфейс");
            }

            synchronized (lock) { tun = established; }
            writeTunConfig();

            if (!TProxyService.TProxyStartService(
                    AppFiles.tunConfigFile(this).getAbsolutePath(),
                    established.getFd())) {
                throw new IllegalStateException("tun2socks не запустился");
            }

            for (int i = 0; i < 20 && !TProxyService.TProxyIsRunning(); i++) {
                Thread.sleep(50);
            }
            if (!TProxyService.TProxyIsRunning()) {
                throw new IllegalStateException("tun2socks не перешёл в рабочее состояние");
            }
            if (!RUNTIME_OWNER.equals(Mobile.owner()) || !Mobile.listenerReady()) {
                throw new IllegalStateException("SOCKS5 остановился во время запуска VPN");
            }

            starting = false;
            active = true;
            notifyState("VPN подключён • весь телефон через Chameleon");
            ChameleonWidget.updateAll(this);
            ChameleonTile.requestRefresh(this);

            while (!stopping && TProxyService.TProxyIsRunning()) {
                Thread.sleep(1000);
            }

            if (!stopping) {
                notifyState("VPN остановлен: TUN transport завершился");
            }
        } catch (PackageManager.NameNotFoundException error) {
            notifyState("VPN ошибка: не удалось исключить Chameleon из собственного VPN");
        } catch (Exception error) {
            if (!stopping) notifyState("VPN ошибка: " + error.getMessage());
        } finally {
            starting = false;
            if (!stopping) shutdown(true);
        }
    }

    private void writeTunConfig() throws Exception {
        String config =
                "tunnel:\n" +
                "  mtu: 1400\n" +
                "  ipv4: 198.18.0.1\n" +
                "  ipv6: 'fd00:1:fd00:1::1'\n" +
                "  icmp: 'reply'\n" +
                "socks5:\n" +
                "  address: 127.0.0.1\n" +
                "  port: 1080\n" +
                "  udp: 'udp'\n" +
                "misc:\n" +
                "  task-stack-size: 86016\n" +
                "  tcp-buffer-size: 65536\n" +
                "  udp-recv-buffer-size: 524288\n" +
                "  max-session-count: 2048\n" +
                "  log-file: null\n" +
                "  log-level: warn\n";

        try (FileOutputStream out = new FileOutputStream(AppFiles.tunConfigFile(this), false)) {
            out.write(config.getBytes(StandardCharsets.UTF_8));
            out.flush();
        }
    }

    private void shutdown(boolean stopService) {
        synchronized (lock) {
            if (stopping && !active && !starting && tun == null
                    && !RUNTIME_OWNER.equals(Mobile.owner())) {
                if (stopService) stopSelf();
                return;
            }
            stopping = true;
            starting = false;
            active = false;

            try {
                if (TProxyService.TProxyIsRunning()) TProxyService.TProxyStopService();
            } catch (Throwable ignored) {}

            Mobile.stopOwned(RUNTIME_OWNER);

            if (tun != null) {
                try { tun.close(); } catch (Exception ignored) {}
                tun = null;
            }
        }

        stopForeground(STOP_FOREGROUND_REMOVE);
        ChameleonWidget.updateAll(this);
        ChameleonTile.requestRefresh(this);
        if (stopService) stopSelf();
    }

    @Override
    public void onRevoke() {
        shutdown(true);
        super.onRevoke();
    }

    @Override
    public void onDestroy() {
        shutdown(false);
        super.onDestroy();
    }

    private void createChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return;
        NotificationChannel channel = new NotificationChannel(
                CHANNEL_ID,
                getString(R.string.channel_vpn),
                NotificationManager.IMPORTANCE_LOW
        );
        channel.setDescription("Полноценный системный VPN Chameleon");
        getSystemService(NotificationManager.class).createNotificationChannel(channel);
    }

    private Notification notification(String text) {
        Intent open = new Intent(this, MainActivity.class);
        PendingIntent openPending = PendingIntent.getActivity(
                this, 0, open,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE
        );

        Intent stop = new Intent(this, ChameleonVpnService.class);
        stop.setAction(ACTION_STOP);
        PendingIntent stopPending = PendingIntent.getService(
                this, 2, stop,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE
        );

        Notification.Builder builder = Build.VERSION.SDK_INT >= Build.VERSION_CODES.O
                ? new Notification.Builder(this, CHANNEL_ID)
                : new Notification.Builder(this);

        return builder
                .setSmallIcon(R.drawable.ic_chameleon)
                .setContentTitle("Chameleon VPN")
                .setContentText(text)
                .setContentIntent(openPending)
                .addAction(new Notification.Action.Builder(
                        R.drawable.ic_chameleon, "Выключить", stopPending).build())
                .setOngoing(true)
                .build();
    }

    private void notifyState(String text) {
        NotificationManager manager = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
        manager.notify(NOTIFICATION_ID, notification(text));
    }
}
