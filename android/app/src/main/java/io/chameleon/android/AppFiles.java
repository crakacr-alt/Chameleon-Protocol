package io.chameleon.android;

import android.content.Context;
import android.content.SharedPreferences;

import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;

/**
 * Centralizes private app state. The JSON config contains the tunnel PSK and
 * therefore never leaves the Android application sandbox.
 */
final class AppFiles {
    private static final String CONFIG_FILE = "client.json";
    private static final String PREFS = "chameleon_ui";
    private static final String KEY_RUNTIME_MODE = "runtime_mode";
    private static final String KEY_LAST_VPN_ERROR = "last_vpn_error";

    private AppFiles() {}

    static File configFile(Context context) {
        return new File(context.getFilesDir(), CONFIG_FILE);
    }

    static File tunConfigFile(Context context) {
        return new File(context.getFilesDir(), "tun2socks.yml");
    }

    static boolean hasConfig(Context context) {
        return configFile(context).isFile();
    }

    static String readConfig(Context context) throws Exception {
        try (FileInputStream input = new FileInputStream(configFile(context));
             ByteArrayOutputStream output = new ByteArrayOutputStream()) {
            byte[] buffer = new byte[8192];
            for (int read; (read = input.read(buffer)) >= 0; ) {
                output.write(buffer, 0, read);
            }
            return new String(output.toByteArray(), StandardCharsets.UTF_8);
        }
    }

    static void writeConfig(Context context, String config) throws Exception {
        try (FileOutputStream output = new FileOutputStream(configFile(context), false)) {
            output.write(config.getBytes(StandardCharsets.UTF_8));
            output.flush();
        }
    }

    static String coreMode(Context context) {
        try {
            return new JSONObject(readConfig(context)).optString("mode", "smart");
        } catch (Exception ignored) {
            return "smart";
        }
    }

    static void setCoreMode(Context context, String mode) throws Exception {
        JSONObject json = new JSONObject(readConfig(context));
        json.put("mode", mode);
        writeConfig(context, json.toString(2) + "\n");
    }

    static String runtimeMode(Context context) {
        SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        return prefs.getString(KEY_RUNTIME_MODE, coreMode(context));
    }

    static void setRuntimeMode(Context context, String mode) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
                .edit().putString(KEY_RUNTIME_MODE, mode).apply();
    }

    static String lastVpnError(Context context) {
        return context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
                .getString(KEY_LAST_VPN_ERROR, "");
    }

    static void setLastVpnError(Context context, String message) {
        context.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
                .edit().putString(KEY_LAST_VPN_ERROR, message == null ? "" : message).apply();
    }

    static void clearLastVpnError(Context context) {
        setLastVpnError(context, "");
    }

    static void forceSmart(Context context) throws Exception {
        setRuntimeMode(context, "smart");
        setCoreMode(context, "smart");
    }

    static String serverLabel(Context context) {
        try {
            JSONObject json = new JSONObject(readConfig(context));
            String value = json.optString("quic_server", "");
            if (value.isEmpty()) value = json.optString("tls_server", "");
            if (value.isEmpty()) value = json.optString("tcp_server", "");
            return value.isEmpty() ? "Сервер не указан" : value;
        } catch (Exception ignored) {
            return "Профиль не импортирован";
        }
    }
}
