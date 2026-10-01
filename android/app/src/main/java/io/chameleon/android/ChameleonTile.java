package io.chameleon.android;

import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.os.Build;
import android.service.quicksettings.Tile;
import android.service.quicksettings.TileService;


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

        Intent toggle = new Intent(this, SmartToggleActivity.class);
        toggle.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        startActivityAndCollapse(toggle);
        refresh();
    }

    private void refresh() {
        Tile tile = getQsTile();
        if (tile == null) return;
        boolean running = ChameleonVpnService.running() || ChameleonVpnService.starting();
        tile.setState(running ? Tile.STATE_ACTIVE : Tile.STATE_INACTIVE);
        String mode = AppFiles.runtimeMode(this);
        String label = "inspector".equals(mode)
                ? "Chameleon Inspector"
                : ("vpn".equals(mode) ? "Chameleon VPN" : "Chameleon Smart");
        tile.setLabel(label);
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
