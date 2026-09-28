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

/**
 * Optional full-device VPN mode.
 *
 * Android owns the TUN interface. hev-socks5-tunnel translates IP packets to
 * the local Chameleon SOCKS5 endpoint, while the shared Go core still owns all
 * Chameleon transport selection and tunnel cryptography.
 */
public final class ChameleonVpnService extends VpnService {
    static final String ACTION_START = "io.chameleon.android.VPN_START";
    static final String ACTION_STOP = "io.chameleon.android.VPN_STOP";

    private static final String CHANNEL_ID = "chameleon_vpn";
    private static final int NOTIFICATION_ID = 1002;

    private static volatile boolean active;

    private ParcelFileDescriptor tun;
    private volatile boolean stopping;

    static boolean running() {
        return active;
    }

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
            stopTunnel();
            return START_NOT_STICKY;
        }

        startForeground(NOTIFICATION_ID, notification("Подключение VPN…"));
        stopping = false;

        new Thread(this::startTunnel, "chameleon-vpn-start").start();
        return START_STICKY;
    }

    private void startTunnel() {
        try {
            // The sidecar service may already own the local SOCKS listener.
            Intent stopSidecar = new Intent(this, ChameleonService.class);
            stopSidecar.setAction(ChameleonService.ACTION_STOP);
            startService(stopSidecar);
            Mobile.stop();

            for (int i = 0; i < 40 && Mobile.running(); i++) {
                Thread.sleep(50);
            }

            AppFiles.setRuntimeMode(this, "vpn");
            AppFiles.setCoreMode(this, "proxy");

            String error = Mobile.start(AppFiles.readConfig(this));
            if (error != null && !error.isEmpty()) {
                throw new IllegalStateException(error);
            }

            Builder builder = new Builder()
                    .setSession("Chameleon VPN")
                    .setMtu(1500)
                    .addAddress("198.18.0.1", 32)
                    .addRoute("0.0.0.0", 0)
                    .addAddress("fd00:1:fd00:1::1", 128)
                    .addRoute("::", 0)
                    .addDnsServer("1.1.1.1")
                    .addDnsServer("8.8.8.8")
                    .setBlocking(true);

            // Prevent a routing loop: Chameleon's own Go sockets and the
            // tun2socks SOCKS connection must use the physical network.
            try {
                builder.addDisallowedApplication(getPackageName());
            } catch (PackageManager.NameNotFoundException e) {
                throw new IllegalStateException("Не удалось исключить Chameleon из собственного VPN", e);
            }

            tun = builder.establish();
            if (tun == null) {
                throw new IllegalStateException("Android не создал TUN-интерфейс");
            }

            writeTunConfig();

            active = true;
            notifyState("VPN подключён • весь телефон через Chameleon");
            ChameleonWidget.updateAll(this);
            ChameleonTile.requestRefresh(this);

            boolean ok = TProxyService.TProxyStartService(
                    AppFiles.tunConfigFile(this).getAbsolutePath(),
                    tun.getFd()
            );

            if (!ok && !stopping) {
                throw new IllegalStateException("tun2socks не запустился");
            }
        } catch (Exception error) {
            if (!stopping) {
                notifyState("VPN ошибка: " + error.getMessage());
            }
        } finally {
            if (!stopping && !TProxyService.TProxyIsRunning()) {
                active = false;
                Mobile.stop();
                closeTun();
                ChameleonWidget.updateAll(this);
                ChameleonTile.requestRefresh(this);
                stopSelf();
            }
        }
    }

    private void writeTunConfig() throws Exception {
        String config =
                "tunnel:\n" +
                "  mtu: 1500\n" +
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
        }
    }

    private synchronized void stopTunnel() {
        stopping = true;
        active = false;
        try {
            TProxyService.TProxyStopService();
        } catch (Throwable ignored) {
        }
        Mobile.stop();
        closeTun();
        stopForeground(STOP_FOREGROUND_REMOVE);
        ChameleonWidget.updateAll(this);
        ChameleonTile.requestRefresh(this);
        stopSelf();
    }

    private void closeTun() {
        ParcelFileDescriptor current = tun;
        tun = null;
        if (current != null) {
            try {
                current.close();
            } catch (Exception ignored) {
            }
        }
    }

    @Override
    public void onRevoke() {
        stopTunnel();
        super.onRevoke();
    }

    @Override
    public void onDestroy() {
        stopTunnel();
        super.onDestroy();
    }

    private void createChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return;
        NotificationChannel channel = new NotificationChannel(
                CHANNEL_ID,
                "Chameleon VPN",
                NotificationManager.IMPORTANCE_LOW
        );
        channel.setDescription("Системный VPN Chameleon");
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
                        R.drawable.ic_chameleon, "Выключить", stopPending
                ).build())
                .setOngoing(true)
                .build();
    }

    private void notifyState(String text) {
        NotificationManager manager = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
        manager.notify(NOTIFICATION_ID, notification(text));
    }
}
