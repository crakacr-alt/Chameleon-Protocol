package io.chameleon.android;

import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.os.Build;
import android.service.quicksettings.Tile;
import android.service.quicksettings.TileService;

import mobile.Mobile;

/** Quick Settings tile: one tap toggles Chameleon in Smart mode. */
public final class ChameleonTile extends TileService {
    @Override
    public void onStartListening() {
        super.onStartListening();
        refresh();
    }

    @Override
    public void onClick() {
        super.onClick();

        if (!AppFiles.hasConfig(this)) {
            Intent open = new Intent(this, MainActivity.class);
            open.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
            startActivityAndCollapse(open);
            return;
        }

        Intent toggle = new Intent(this, ChameleonService.class);
        toggle.setAction(ChameleonService.ACTION_SMART_TOGGLE);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && !Mobile.running()) {
            startForegroundService(toggle);
        } else {
            startService(toggle);
        }
        refresh();
    }

    private void refresh() {
        Tile tile = getQsTile();
        if (tile == null) return;
        boolean running = Mobile.running();
        tile.setState(running ? Tile.STATE_ACTIVE : Tile.STATE_INACTIVE);
        tile.setLabel("Chameleon Smart");
        tile.updateTile();
    }

    static void requestRefresh(Context context) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            TileService.requestListeningState(
                    context,
                    new ComponentName(context, ChameleonTile.class)
            );
        }
    }
}
