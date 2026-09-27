package io.chameleon.android;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.content.Intent;
import android.os.Build;
import android.os.IBinder;

import mobile.Mobile;

/**
 * Foreground owner for the Go networking runtime.
 *
 * Android owns process/lifecycle policy while the shared Go core owns SOCKS,
 * adaptive routing, QUIC/TLS/TCP and protocol state.
 */
public final class ChameleonService extends Service {
    static final String ACTION_START = "io.chameleon.android.START";
    static final String ACTION_STOP = "io.chameleon.android.STOP";

    private static final String CHANNEL_ID = "chameleon_connection";
    private static final int NOTIFICATION_ID = 1001;

    @Override
    public void onCreate() {
        super.onCreate();
        createChannel();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && ACTION_STOP.equals(intent.getAction())) {
            Mobile.stop();
            stopForeground(STOP_FOREGROUND_REMOVE);
            stopSelf();
            return START_NOT_STICKY;
        }

        startForeground(NOTIFICATION_ID, notification("Подключение…"));

        try {
            String config = AppFiles.readConfig(this);
            String error = Mobile.start(config);
            if (error != null && !error.isEmpty()) {
                notifyState("Ошибка: " + error);
                stopSelf();
                return START_NOT_STICKY;
            }
            notifyState("Подключено • SOCKS5 127.0.0.1:1080");
            return START_STICKY;
        } catch (Exception error) {
            notifyState("Ошибка профиля: " + error.getMessage());
            stopSelf();
            return START_NOT_STICKY;
        }
    }

    @Override
    public void onDestroy() {
        Mobile.stop();
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
        channel.setDescription("Состояние локального Chameleon proxy");
        getSystemService(NotificationManager.class).createNotificationChannel(channel);
    }

    private Notification notification(String text) {
        Intent open = new Intent(this, MainActivity.class);
        android.app.PendingIntent pending = android.app.PendingIntent.getActivity(
                this,
                0,
                open,
                android.app.PendingIntent.FLAG_UPDATE_CURRENT | android.app.PendingIntent.FLAG_IMMUTABLE
        );

        Notification.Builder builder = Build.VERSION.SDK_INT >= Build.VERSION_CODES.O
                ? new Notification.Builder(this, CHANNEL_ID)
                : new Notification.Builder(this);

        return builder
                .setSmallIcon(R.drawable.ic_chameleon)
                .setContentTitle("Chameleon")
                .setContentText(text)
                .setContentIntent(pending)
                .setOngoing(Mobile.running())
                .build();
    }

    private void notifyState(String text) {
        NotificationManager manager = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
        manager.notify(NOTIFICATION_ID, notification(text));
    }
}
