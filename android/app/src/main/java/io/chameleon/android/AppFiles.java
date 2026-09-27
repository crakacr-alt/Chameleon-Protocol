package io.chameleon.android;

import android.content.Context;

import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.nio.charset.StandardCharsets;

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
