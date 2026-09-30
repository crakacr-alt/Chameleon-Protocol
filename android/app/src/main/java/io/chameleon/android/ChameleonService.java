package io.chameleon.android;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.app.Service;
import android.content.Intent;
import android.os.Build;
import android.os.IBinder;
import android.util.Log;

import mobile.Mobile;

public final class ChameleonService extends Service {
    private static final String TAG = "ChameleonService";
    static final String ACTION_START = "io.chameleon.android.START";
    static final String ACTION_STOP = "io.chameleon.android.STOP";
    static final String ACTION_SMART_TOGGLE = "io.chameleon.android.SMART_TOGGLE";
    static final String RUNTIME_OWNER = "sidecar";

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

        if (ACTION_SMART_TOGGLE.equals(action) && runtimeRunning()) {
            stopRuntime();
            return START_NOT_STICKY;
        }

        startForeground(NOTIFICATION_ID, notification("Подключение…"));

        try {
            if (ACTION_SMART_TOGGLE.equals(action)) {
                AppFiles.forceSmart(this);
            }

            String error = Mobile.startOwned(AppFiles.readConfig(this), RUNTIME_OWNER);
            if (error != null && !error.isEmpty()) {
                Log.e(TAG, "runtime start failed: " + error);
                notifyState("Ошибка: " + error);
                stopSelf();
                return START_NOT_STICKY;
            }
            if (!Mobile.listenerReady()) {
                Log.e(TAG, "listener stopped during startup; stage=" + Mobile.stage()
                        + " error=" + Mobile.lastError());
                notifyState("Ошибка: SOCKS5 listener не готов");
                Mobile.stopOwned(RUNTIME_OWNER);
                stopSelf();
                return START_NOT_STICKY;
            }

            String mode = AppFiles.coreMode(this);
            notifyState("Подключено • " + mode.toUpperCase() + " • SOCKS5 127.0.0.1:1080");
            ChameleonWidget.updateAll(this);
            ChameleonTile.requestRefresh(this);
            return START_STICKY;
        } catch (Exception error) {
            Log.e(TAG, "startup failed: stage=" + Mobile.stage()
                    + " error=" + error.getMessage(), error);
            notifyState("Ошибка профиля: " + error.getMessage());
            stopSelf();
            return START_NOT_STICKY;
        }
    }

    private boolean runtimeRunning() {
        return RUNTIME_OWNER.equals(Mobile.owner()) && Mobile.listenerReady();
    }

    private void stopRuntime() {
        Mobile.stopOwned(RUNTIME_OWNER);
        stopForeground(STOP_FOREGROUND_REMOVE);
        stopSelf();
        ChameleonWidget.updateAll(this);
        ChameleonTile.requestRefresh(this);
    }

    @Override
    public void onDestroy() {
        Mobile.stopOwned(RUNTIME_OWNER);
        ChameleonWidget.updateAll(this);
        ChameleonTile.requestRefresh(this);
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) { return null; }

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
        toggle.setAction(runtimeRunning() ? ACTION_STOP : ACTION_SMART_TOGGLE);
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
                        runtimeRunning() ? "Выключить" : "Smart",
                        togglePending
                ).build())
                .setOngoing(runtimeRunning())
                .build();
    }

    private void notifyState(String text) {
        NotificationManager manager = (NotificationManager) getSystemService(NOTIFICATION_SERVICE);
        manager.notify(NOTIFICATION_ID, notification(text));
    }
}
