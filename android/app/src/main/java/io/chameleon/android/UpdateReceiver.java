package io.chameleon.android;

import android.app.DownloadManager;
import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;
import android.os.Build;

/**
 * Turns a completed DownloadManager job into an explicit installation prompt.
 * Android does not allow a normal app to silently replace itself.
 */
public final class UpdateReceiver extends BroadcastReceiver {
    private static final String CHANNEL_ID = "chameleon_updates";
    private static final int NOTIFICATION_ID = 2201;

    @Override
    public void onReceive(Context context, Intent intent) {
        if (!DownloadManager.ACTION_DOWNLOAD_COMPLETE.equals(intent.getAction())) return;

        long completed = intent.getLongExtra(DownloadManager.EXTRA_DOWNLOAD_ID, -1);
        long pending = context.getSharedPreferences(UpdateRepository.PREFS, Context.MODE_PRIVATE)
                .getLong("pending_download_id", -2);
        if (completed <= 0 || completed != pending) return;

        createChannel(context);

        Intent install = new Intent(context, InstallerActivity.class);
        install.putExtra("download_id", completed);
        PendingIntent pendingIntent = PendingIntent.getActivity(
                context,
                2201,
                install,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE
        );

        Notification.Builder builder = Build.VERSION.SDK_INT >= Build.VERSION_CODES.O
                ? new Notification.Builder(context, CHANNEL_ID)
                : new Notification.Builder(context);

        Notification notification = builder
                .setSmallIcon(R.drawable.ic_chameleon)
                .setContentTitle("Обновление Chameleon скачано")
                .setContentText("Нажмите, чтобы открыть системную установку")
                .setAutoCancel(true)
                .setContentIntent(pendingIntent)
                .build();

        ((NotificationManager) context.getSystemService(Context.NOTIFICATION_SERVICE))
                .notify(NOTIFICATION_ID, notification);
    }

    private void createChannel(Context context) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return;
        NotificationChannel channel = new NotificationChannel(
                CHANNEL_ID,
                context.getString(R.string.channel_updates),
                NotificationManager.IMPORTANCE_DEFAULT
        );
        ((NotificationManager) context.getSystemService(Context.NOTIFICATION_SERVICE))
                .createNotificationChannel(channel);
    }
}
