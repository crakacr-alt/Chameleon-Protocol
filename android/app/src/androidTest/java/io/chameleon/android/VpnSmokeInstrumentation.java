package io.chameleon.android;

import android.app.Instrumentation;
import android.content.Context;
import android.content.Intent;
import android.net.VpnService;
import android.os.Bundle;
import java.io.File;
import java.io.FileOutputStream;
import android.app.Activity;
import android.graphics.Bitmap;
import java.lang.reflect.Field;
import java.lang.reflect.Method;
import java.util.List;
import java.util.Map;
import android.view.accessibility.AccessibilityNodeInfo;

/** Emulator/device smoke check; profile stays in the target app's private files. */
public final class VpnSmokeInstrumentation extends Instrumentation {
    private String mode;
    private int holdSeconds;

    @Override public void onCreate(Bundle arguments) {
        super.onCreate(arguments);
        mode = arguments == null ? "vpn" : arguments.getString("mode", "vpn");
        holdSeconds = arguments == null ? 0 : Integer.parseInt(arguments.getString("hold_seconds", "0"));
        start();
    }

    @Override public void onStart() {
        Bundle result = new Bundle();
        try {
            Context context = getTargetContext();
            if ("stop".equals(mode)) {
                ChameleonVpnService.requestStop(context);
                for (int i = 0; i < 40 && (ChameleonVpnService.running()
                        || ChameleonVpnService.starting()); i++) Thread.sleep(100);
                if (ChameleonVpnService.running()) throw new IllegalStateException("VPN did not stop");
            } else {
                if (VpnService.prepare(context) != null)
                    throw new IllegalStateException("Grant Android VPN consent before running this test");
                if (!AppFiles.hasConfig(context))
                    throw new IllegalStateException("Import a test profile first");
                Intent intent = new Intent(context, ChameleonVpnService.class);
                intent.setAction(ChameleonVpnService.ACTION_START);
                intent.putExtra(ChameleonVpnService.EXTRA_MODE, mode);
                context.startForegroundService(intent);
                for (int i = 0; i < 900 && !ChameleonVpnService.running(); i++) {
                    Thread.sleep(100);
                    String error = AppFiles.lastVpnError(context);
                    if (!error.isEmpty()) throw new IllegalStateException(error);
                }
                if (!ChameleonVpnService.running()) throw new IllegalStateException("VPN startup timed out");
                if (!mobile.Mobile.listenerReady()) throw new IllegalStateException("SOCKS listener is not ready");
                if ("inspector".equals(mode)) {
                    File pcap = AppFiles.captureFile(context);
                    if (!pcap.isFile() || pcap.length() < 24)
                        throw new IllegalStateException("PCAP writer did not create a valid header");
                }
                result.putString("socks", "ready");
                result.putString("stage", mobile.Mobile.stage());
                if (holdSeconds > 0) {
                    result.putString("result", "READY");
                    sendStatus(1, result);
                    File stop = new File(context.getFilesDir(), "smoke-stop");
                    for (int i = 0; i < Math.min(holdSeconds, 300) * 10 && !stop.exists(); i++)
                        Thread.sleep(100);
                    if (!ChameleonVpnService.running()) throw new IllegalStateException("VPN stopped during traffic test");
                    if ("inspector".equals(mode)) verifyInspectorScreen(context, result);
                    ChameleonVpnService.requestStop(context);
                }
            }
            result.putString("mode", mode);
            result.putString("result", "PASS");
            finish(-1, result);
        } catch (Exception error) {
            ChameleonVpnService.requestStop(getTargetContext());
            result.putString("result", "FAIL");
            result.putString("error", error.toString());
            finish(0, result);
        }
    }

    private void verifyInspectorScreen(Context context, Bundle result) throws Exception {
        // A stale System UI ANR dialog from emulator boot can obscure the app.
        // Dismiss that specific unrelated dialog, never an ANR for Chameleon.
        AccessibilityNodeInfo root = getUiAutomation().getRootInActiveWindow();
        if (root != null && !root.findAccessibilityNodeInfosByText("System UI isn't responding").isEmpty()) {
            for (AccessibilityNodeInfo node : root.findAccessibilityNodeInfosByText("Close app")) {
                for (int i = 0; i < 5 && node != null && !node.isClickable(); i++) node = node.getParent();
                if (node != null) node.performAction(AccessibilityNodeInfo.ACTION_CLICK);
            }
            Thread.sleep(1000);
        }
        Activity activity = startActivitySync(new Intent(context, InspectorActivity.class)
                .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK));
        Field packetField = InspectorActivity.class.getDeclaredField("packets");
        Field flowField = InspectorActivity.class.getDeclaredField("flows");
        Method refresh = InspectorActivity.class.getDeclaredMethod("refreshInspector");
        packetField.setAccessible(true); flowField.setAccessible(true);
        refresh.setAccessible(true);
        final Throwable[] failure = {null};
        runOnMainSync(() -> {
            try {
                java.util.ArrayList<android.view.View> controls = new java.util.ArrayList<>();
                activity.getWindow().getDecorView().findViewsWithText(controls,
                        "Показать отдельные пакеты", android.view.View.FIND_VIEWS_WITH_TEXT);
                if (controls.isEmpty() || !controls.get(0).performClick())
                    throw new Exception("Packet view button did not respond");
                refresh.invoke(activity);
                List<?> packets = (List<?>) packetField.get(activity);
                Map<?, ?> flows = (Map<?, ?>) flowField.get(activity);
                if (packets.isEmpty() || flows.isEmpty()) throw new Exception("Inspector parsed no traffic");
                result.putString("inspector_packets", Integer.toString(packets.size()));
                result.putString("inspector_flows", Integer.toString(flows.size()));
            } catch (Throwable error) { failure[0] = error; }
        });
        if (failure[0] != null) throw new Exception(failure[0]);
        waitForIdleSync();
        saveScreenshot(context, "inspector-packets.png");
        List<?> packets = (List<?>) packetField.get(activity);
        Method showPacket = InspectorActivity.class.getDeclaredMethod("showPacket", packets.get(0).getClass());
        showPacket.setAccessible(true);
        runOnMainSync(() -> {
            try { showPacket.invoke(activity, packets.get(0)); }
            catch (Throwable error) { failure[0] = error; }
        });
        if (failure[0] != null) throw new Exception(failure[0]);
        waitForIdleSync();
        saveScreenshot(context, "inspector-packet-detail.png");
    }

    private void saveScreenshot(Context context, String name) throws Exception {
        Thread.sleep(1000); // allow SurfaceFlinger to present the UI frame
        AccessibilityNodeInfo window = getUiAutomation().getRootInActiveWindow();
        if (window == null || !context.getPackageName().contentEquals(window.getPackageName()))
            throw new Exception("Another Android window obscures Inspector: "
                    + (window == null ? "null" : window.getPackageName()));
        Bitmap screenshot = getUiAutomation().takeScreenshot();
        if (screenshot == null) throw new Exception("Inspector screenshot is unavailable");
        try (FileOutputStream out = new FileOutputStream(new File(context.getFilesDir(), name))) {
            screenshot.compress(Bitmap.CompressFormat.PNG, 100, out);
        }
        screenshot.recycle();
    }
}
