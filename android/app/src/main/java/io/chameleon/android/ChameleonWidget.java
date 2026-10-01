package io.chameleon.android;

import android.app.PendingIntent;
import android.appwidget.AppWidgetManager;
import android.appwidget.AppWidgetProvider;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.os.Build;
import android.widget.RemoteViews;


/** One-tap home-screen widget for Chameleon Smart mode. */
public final class ChameleonWidget extends AppWidgetProvider {
    private static final String ACTION_TOGGLE = "io.chameleon.android.WIDGET_TOGGLE";

    @Override
    public void onUpdate(Context context, AppWidgetManager manager, int[] appWidgetIds) {
        for (int id : appWidgetIds) {
            manager.updateAppWidget(id, views(context));
        }
    }

    @Override
    public void onReceive(Context context, Intent intent) {
        if (ACTION_TOGGLE.equals(intent.getAction())) {
            Intent toggle = new Intent(context, SmartToggleActivity.class);
            toggle.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            context.startActivity(toggle);
            updateAll(context);
        }
        super.onReceive(context, intent);
    }

    private static RemoteViews views(Context context) {
        boolean running = ChameleonVpnService.running() || ChameleonVpnService.starting();
        String mode = AppFiles.runtimeMode(context);
        String activeLabel = "inspector".equals(mode)
                ? "INSPECTOR ON"
                : ("vpn".equals(mode) ? "VPN ON" : "SMART ON");
        RemoteViews views = new RemoteViews(context.getPackageName(), R.layout.widget_chameleon);
        views.setTextViewText(
                R.id.widget_state,
                running ? activeLabel : "SMART OFF"
        );

        Intent toggle = new Intent(context, SmartToggleActivity.class);
        PendingIntent pending = PendingIntent.getActivity(
                context, 4, toggle,
                PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE
        );
        views.setOnClickPendingIntent(R.id.widget_button, pending);
        return views;
    }

    static void updateAll(Context context) {
        AppWidgetManager manager = AppWidgetManager.getInstance(context);
        ComponentName name = new ComponentName(context, ChameleonWidget.class);
        int[] ids = manager.getAppWidgetIds(name);
        for (int id : ids) {
            manager.updateAppWidget(id, views(context));
        }
    }
}
