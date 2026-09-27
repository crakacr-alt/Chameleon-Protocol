package io.chameleon.android;

import android.app.Activity;
import android.app.AlertDialog;
import android.app.DownloadManager;
import android.content.Intent;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.provider.Settings;

import java.io.InputStream;
import java.security.MessageDigest;
import java.util.Locale;

/**
 * Verifies the downloaded APK checksum and hands installation to Android's
 * package installer. Silent self-installation is intentionally not attempted.
 */
public final class InstallerActivity extends Activity {
    private static final int REQUEST_UNKNOWN_SOURCES = 3001;
    private long downloadId;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        downloadId = getIntent().getLongExtra(
                "download_id",
                getSharedPreferences(UpdateRepository.PREFS, MODE_PRIVATE)
                        .getLong("pending_download_id", -1)
        );
        continueInstall();
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == REQUEST_UNKNOWN_SOURCES) {
            continueInstall();
        }
    }

    private void continueInstall() {
        if (downloadId <= 0) {
            finishWithError("Файл обновления не найден.");
            return;
        }

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O
                && !getPackageManager().canRequestPackageInstalls()) {
            Intent permission = new Intent(
                    Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
                    Uri.parse("package:" + getPackageName())
            );
            startActivityForResult(permission, REQUEST_UNKNOWN_SOURCES);
            return;
        }

        DownloadManager manager = (DownloadManager) getSystemService(DOWNLOAD_SERVICE);
        Uri apk = manager.getUriForDownloadedFile(downloadId);
        if (apk == null) {
            finishWithError("Android не нашёл скачанный APK.");
            return;
        }

        String expected = getSharedPreferences(UpdateRepository.PREFS, MODE_PRIVATE)
                .getString("pending_sha256", "");
        if (expected != null && !expected.trim().isEmpty()) {
            try {
                String actual = sha256(apk);
                if (!expected.trim().equalsIgnoreCase(actual)) {
                    finishWithError("SHA-256 APK не совпадает с индексом GitHub. Установка отменена.");
                    return;
                }
            } catch (Exception error) {
                finishWithError("Не удалось проверить SHA-256: " + error.getMessage());
                return;
            }
        }

        Intent install = new Intent(Intent.ACTION_VIEW);
        install.setDataAndType(apk, "application/vnd.android.package-archive");
        install.addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION);
        install.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK);

        try {
            startActivity(install);
            finish();
        } catch (Exception error) {
            finishWithError("Не удалось открыть установщик Android: " + error.getMessage());
        }
    }

    private String sha256(Uri uri) throws Exception {
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        try (InputStream input = getContentResolver().openInputStream(uri)) {
            if (input == null) throw new IllegalStateException("cannot open downloaded APK");
            byte[] buffer = new byte[8192];
            for (int read; (read = input.read(buffer)) >= 0; ) {
                digest.update(buffer, 0, read);
            }
        }

        StringBuilder hex = new StringBuilder();
        for (byte value : digest.digest()) {
            hex.append(String.format(Locale.ROOT, "%02x", value & 0xff));
        }
        return hex.toString();
    }

    private void finishWithError(String message) {
        new AlertDialog.Builder(this)
                .setTitle("Обновление Chameleon")
                .setMessage(message)
                .setPositiveButton("OK", (dialog, which) -> finish())
                .setOnCancelListener(dialog -> finish())
                .show();
    }
}
