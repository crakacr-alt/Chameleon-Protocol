package io.chameleon.android;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.os.Build;
import android.os.IBinder;

import mobile.Mobile;

/**
 * Foreground owner for the local SOCKS sidecar.
 *
 * Smart and Proxy sidecar modes do not claim Android's VpnService slot.
 * ChameleonVpnService owns the optional full-device TUN mode.
 */
public final class ChameleonService extends Service {
    static final String ACTION_START = "io.chameleon.android.START";
    static final String ACTION_STOP = "io.chameleon.android.STOP";
    static final String ACTION_SMART_TOGGLE = "io.chameleon.android.SMART_TOGGLE";

    private static final String CHANNEL_ID = "chameleon_connection";
    private static final int NOTIFICATION_ID = 1001;

    @Override
    public void onCreate() {
        super.onCreate();
        createChannel();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        String action = intent == null ? ACTION_START : intent.getAction();

        if (ACTION_STOP.equals(action)) {
            stopRuntime();
            return START_NOT_STICKY;
        }

        if (ACTION_SMART_TOGGLE.equals(action) && Mobile.running()) {
            stopRuntime();
            return START_NOT_STICKY;
        }

        startForeground(NOTIFICATION_ID, notification("Подключение…"));

        try {
            if (ACTION_SMART_TOGGLE.equals(action)) {
                if (ChameleonVpnService.running()) {
                    ChameleonVpnService.requestStop(this);
                }
                AppFiles.forceSmart(this);
            }

            String config = AppFiles.readConfig(this);
            String error = Mobile.start(config);
            if (error != null && !error.isEmpty()) {
                notifyState("Ошибка: " + error, false);
                stopSelf();
                return START_NOT_STICKY;
            }

            String mode = AppFiles.coreMode(this);
            notifyState("Подключено • " + mode.toUpperCase() + " • SOCKS5 127.0.0.1:1080", true);
            ChameleonWidget.updateAll(this);
            ChameleonTile.requestRefresh(this);
            return START_STICKY;
        } catch (Exception error) {
            notifyState("Ошибка профиля: " + error.getMessage(), false);
            stopSelf();
            return START_NOT_STICKY;
        }
    }

    private void stopRuntime() {
        Mobile.stop();
        stopForeground(STOP_FOREGROUND_REMOVE);
        stopSelf();
        ChameleonWidget.updateAll(this);
        ChameleonTile.requestRefresh(this);
    }

    @Override
    public void onDestroy() {
        Mobile.stop();
        ChameleonWidget.updateAll(this);
        ChameleonTile.requestRefresh(this);
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    private void createChannel() {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return;
        NotificationChannel channel = new NotificationChannel(
                CHANNEL_ID,
                getString(R.string.channel_connection),
                NotificationManager.IMPORTANCE_LOW
        );
        channel.setDescription("Состояние Chameleon");
        getSystemService(NotificationManager.class).createNotificationChannel(channel);
    }

    private Notification notification(String text) {
        Intent open = new Intent(this, MainActivity.class);
        PendingIntent pending = PendingIntent.getActivity(
                this, 0, open,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE
        );

        Intent toggle = new Intent(this, ChameleonService.class);
        toggle.setAction(Mobile.running() ? ACTION_STOP : ACTION_SMART_TOGGLE);
        PendingIntent togglePending = PendingIntent.getService(
                this, 1, toggle,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE
        );

        Notification.Builder builder = Build.VERSION.SDK_INT >= Build.VERSION_CODES.O
                ? new Notification.Builder(this, CHANNEL_ID)
                : new Notification.Builder(this);

        return builder
                .setSmallIcon(R.drawable.ic_chameleon)
                .setContentTitle("Chameleon")
                .setContentText(text)
                .setContentIntent(pending)
                .addAction(new Notification.Action.Builder(
                        R.drawable.ic_chameleon,
                        Mobile.running() ? "Выключить" : "Smart",
                        togglePending
                ).build())
                .setOngoing(Mobile.running())
                .build();
    }

    private void notifyState(String text, boolean ongoing) {
        NotificationManager manager = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
        Notification note = notification(text);
        manager.notify(NOTIFICATION_ID, note);
    }
}
