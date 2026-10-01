package io.chameleon.android;

import android.app.Activity;
import android.content.Intent;
import android.net.VpnService;
import android.os.Build;
import android.os.Bundle;
import android.widget.Toast;

import mobile.Mobile;

/**
 * Small user-visible bridge used by the Quick Settings tile and home widget.
 * It requests the normal Android VPN permission when needed and then starts
 * the same full-device Smart VpnService as MainActivity.
 */
public final class SmartToggleActivity extends Activity {
    private static final int REQUEST_VPN = 4101;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        handleToggle();
    }

    private void handleToggle() {
        if (!AppFiles.hasConfig(this)) {
            openMain();
            return;
        }

        if (ChameleonVpnService.running() || ChameleonVpnService.starting()) {
            ChameleonVpnService.requestStop(this);
            finish();
            return;
        }

        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) {
            Toast.makeText(this, "Smart VPN требует Android 10 или новее", Toast.LENGTH_LONG).show();
            openMain();
            return;
        }

        Intent permission = VpnService.prepare(this);
        if (permission != null) {
            startActivityForResult(permission, REQUEST_VPN);
            return;
        }

        startSmart();
    }

    private void startSmart() {
        try {
            // Remove any legacy local-SOCKS runtime left by an older APK.
            stopService(new Intent(this, ChameleonService.class));
            Mobile.stopOwned(ChameleonService.RUNTIME_OWNER);

            AppFiles.forceSmart(this);
            Intent start = new Intent(this, ChameleonVpnService.class);
            start.setAction(ChameleonVpnService.ACTION_START);
            start.putExtra(ChameleonVpnService.EXTRA_MODE, "smart");
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
                startForegroundService(start);
            } else {
                startService(start);
            }
        } catch (Exception error) {
            Toast.makeText(this, "Не удалось запустить Smart: " + error.getMessage(), Toast.LENGTH_LONG).show();
        }
        finish();
    }

    private void openMain() {
        Intent open = new Intent(this, MainActivity.class);
        open.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);
        startActivity(open);
        finish();
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode != REQUEST_VPN) return;
        if (resultCode == RESULT_OK) {
            startSmart();
        } else {
            Toast.makeText(this, "Разрешение VPN не выдано", Toast.LENGTH_SHORT).show();
            finish();
        }
    }
}
