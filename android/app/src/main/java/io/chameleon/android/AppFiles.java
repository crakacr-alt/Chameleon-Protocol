package io.chameleon.android;

import android.content.Context;

import org.json.JSONObject;

import java.io.File;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;

/**
 * Centralizes private app files.
 *
 * The Chameleon config contains a PSK, so it is deliberately kept inside the
 * Android application sandbox and never written to shared Downloads storage.
 */
final class AppFiles {
    private static final String CONFIG_FILE = "client.json";

    private AppFiles() {}

    static File configFile(Context context) {
        return new File(context.getFilesDir(), CONFIG_FILE);
    }

    static boolean hasConfig(Context context) {
        return configFile(context).isFile();
    }

    static String readConfig(Context context) throws Exception {
        return Files.readString(configFile(context).toPath(), StandardCharsets.UTF_8);
    }

    static void writeConfig(Context context, String config) throws Exception {
        Files.writeString(configFile(context).toPath(), config, StandardCharsets.UTF_8);
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
