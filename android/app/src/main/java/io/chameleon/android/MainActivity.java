package io.chameleon.android;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.net.Uri;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;
import android.view.Gravity;
import android.view.View;
import android.widget.ArrayAdapter;
import android.widget.Button;
import android.widget.CheckBox;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.Spinner;
import android.widget.TextView;
import android.widget.Toast;

import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.InputStream;
import java.nio.charset.StandardCharsets;

import mobile.Mobile;

/**
 * Minimal main screen inspired by simple one-button proxy clients.
 *
 * It intentionally exposes only decisions a normal user needs: connect,
 * profile, mode and updates. Transport details stay in the shared adaptive core.
 */
public final class MainActivity extends Activity {
    private static final int PICK_PROFILE = 2001;
    private static final int REQUEST_NOTIFICATIONS = 2002;

    private final Handler handler = new Handler(Looper.getMainLooper());

    private TextView stateText;
    private TextView serverText;
    private TextView detailText;
    private Button powerButton;
    private Spinner modeSpinner;
    private CheckBox autoUpdate;

    private final Runnable refresh = new Runnable() {
        @Override
        public void run() {
            refreshState();
            handler.postDelayed(this, 1000);
        }
    };

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        setContentView(buildUi());
        askNotificationPermission();
        refreshState();
        checkUpdates(false);
    }

    @Override
    protected void onResume() {
        super.onResume();
        handler.removeCallbacks(refresh);
        handler.post(refresh);
    }

    @Override
    protected void onPause() {
        handler.removeCallbacks(refresh);
        super.onPause();
    }

    private View buildUi() {
        ScrollView scroll = new ScrollView(this);
        scroll.setFillViewport(true);
        scroll.setBackgroundColor(color(R.color.bg));

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setGravity(Gravity.CENTER_HORIZONTAL);
        root.setPadding(dp(24), dp(28), dp(24), dp(28));
        scroll.addView(root, new ScrollView.LayoutParams(-1, -1));

        TextView title = label("CHAMELEON", 22, R.color.textPrimary);
        title.setTypeface(Typeface.DEFAULT_BOLD);
        root.addView(title);

        TextView version = label(
                "Android " + BuildConfig.VERSION_NAME + " • Protocol " + Mobile.version(),
                12,
                R.color.textSecondary
        );
        LinearLayout.LayoutParams versionParams = wrap();
        versionParams.topMargin = dp(6);
        root.addView(version, versionParams);

        TextView coexist = label("● Tailscale совместим • Proxy mode", 13, R.color.success);
        LinearLayout.LayoutParams coexistParams = wrap();
        coexistParams.topMargin = dp(20);
        root.addView(coexist, coexistParams);

        powerButton = new Button(this);
        powerButton.setText("⏻");
        powerButton.setTextSize(52);
        powerButton.setTextColor(color(R.color.textPrimary));
        powerButton.setAllCaps(false);
        powerButton.setGravity(Gravity.CENTER);
        powerButton.setBackground(circle(color(R.color.accent)));
        powerButton.setOnClickListener(v -> toggleConnection());
        LinearLayout.LayoutParams powerParams = new LinearLayout.LayoutParams(dp(164), dp(164));
        powerParams.topMargin = dp(28);
        root.addView(powerButton, powerParams);

        stateText = label("Отключено", 20, R.color.textPrimary);
        stateText.setTypeface(Typeface.DEFAULT_BOLD);
        LinearLayout.LayoutParams stateParams = wrap();
        stateParams.topMargin = dp(18);
        root.addView(stateText, stateParams);

        detailText = label("SOCKS5 127.0.0.1:1080", 13, R.color.textSecondary);
        LinearLayout.LayoutParams detailParams = wrap();
        detailParams.topMargin = dp(5);
        root.addView(detailText, detailParams);

        LinearLayout card = panel();
        LinearLayout.LayoutParams cardParams = new LinearLayout.LayoutParams(-1, -2);
        cardParams.topMargin = dp(26);
        root.addView(card, cardParams);

        TextView serverCaption = label("СЕРВЕР", 11, R.color.textSecondary);
        card.addView(serverCaption);
        serverText = label("Профиль не импортирован", 16, R.color.textPrimary);
        LinearLayout.LayoutParams serverParams = wrap();
        serverParams.topMargin = dp(5);
        card.addView(serverText, serverParams);

        TextView modeCaption = label("РЕЖИМ", 11, R.color.textSecondary);
        LinearLayout.LayoutParams modeCaptionParams = wrap();
        modeCaptionParams.topMargin = dp(18);
        card.addView(modeCaption, modeCaptionParams);

        modeSpinner = new Spinner(this);
        String[] modes = {
                "Smart — direct + Chameleon",
                "Proxy — только Chameleon"
        };
        ArrayAdapter<String> adapter = new ArrayAdapter<>(
                this,
                android.R.layout.simple_spinner_dropdown_item,
                modes
        );
        modeSpinner.setAdapter(adapter);
        card.addView(modeSpinner, new LinearLayout.LayoutParams(-1, dp(52)));

        Button importButton = secondaryButton("Импортировать профиль");
        importButton.setOnClickListener(v -> chooseProfile());
        LinearLayout.LayoutParams importParams = new LinearLayout.LayoutParams(-1, dp(52));
        importParams.topMargin = dp(18);
        root.addView(importButton, importParams);

        Button versionsButton = secondaryButton("Версии и изменения");
        versionsButton.setOnClickListener(v -> startActivity(new Intent(this, VersionActivity.class)));
        LinearLayout.LayoutParams versionsParams = new LinearLayout.LayoutParams(-1, dp(52));
        versionsParams.topMargin = dp(10);
        root.addView(versionsButton, versionsParams);

        autoUpdate = new CheckBox(this);
        autoUpdate.setText("Автоматически проверять и скачивать обновления");
        autoUpdate.setTextColor(color(R.color.textSecondary));
        autoUpdate.setChecked(getPreferences(MODE_PRIVATE).getBoolean("auto_update", true));
        autoUpdate.setOnCheckedChangeListener((button, checked) ->
                getPreferences(MODE_PRIVATE).edit().putBoolean("auto_update", checked).apply()
        );
        LinearLayout.LayoutParams autoParams = new LinearLayout.LayoutParams(-1, -2);
        autoParams.topMargin = dp(14);
        root.addView(autoUpdate, autoParams);

        Button doctorButton = secondaryButton("Проверить конфигурацию");
        doctorButton.setOnClickListener(v -> showDiagnostics());
        LinearLayout.LayoutParams doctorParams = new LinearLayout.LayoutParams(-1, dp(48));
        doctorParams.topMargin = dp(8);
        root.addView(doctorButton, doctorParams);

        TextView note = label(
                "Этот Android alpha работает как локальный SOCKS5-клиент и не занимает системный VPN-слот. Поэтому Tailscale можно оставить включённым.",
                12,
                R.color.textSecondary
        );
        note.setGravity(Gravity.CENTER);
        LinearLayout.LayoutParams noteParams = new LinearLayout.LayoutParams(-1, -2);
        noteParams.topMargin = dp(22);
        root.addView(note, noteParams);

        return scroll;
    }

    private void toggleConnection() {
        if (Mobile.running()) {
            Intent stop = new Intent(this, ChameleonService.class);
            stop.setAction(ChameleonService.ACTION_STOP);
            startService(stop);
            return;
        }
        if (!AppFiles.hasConfig(this)) {
            Toast.makeText(this, "Сначала импортируйте client-profile.txt", Toast.LENGTH_LONG).show();
            chooseProfile();
            return;
        }

        Intent start = new Intent(this, ChameleonService.class);
        start.setAction(ChameleonService.ACTION_START);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(start);
        } else {
            startService(start);
        }
    }

    private void chooseProfile() {
        Intent intent = new Intent(Intent.ACTION_OPEN_DOCUMENT);
        intent.addCategory(Intent.CATEGORY_OPENABLE);
        intent.setType("*/*");
        startActivityForResult(intent, PICK_PROFILE);
    }

    @Override
    protected void onActivityResult(int requestCode, int resultCode, Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode != PICK_PROFILE || resultCode != RESULT_OK || data == null) return;

        Uri uri = data.getData();
        if (uri == null) return;

        try (InputStream input = getContentResolver().openInputStream(uri)) {
            if (input == null) throw new IllegalStateException("Не удалось открыть файл");
            ByteArrayOutputStream bytes = new ByteArrayOutputStream();
            byte[] buffer = new byte[8192];
            for (int read; (read = input.read(buffer)) >= 0; ) {
                bytes.write(buffer, 0, read);
            }
            String profile = bytes.toString(StandardCharsets.UTF_8);
            String mode = modeSpinner.getSelectedItemPosition() == 1 ? "proxy" : "smart";
            String stateDir = new java.io.File(getFilesDir(), "state").getAbsolutePath();
            String config = Mobile.buildConfig(profile, stateDir, mode);
            if (config.startsWith("ERROR:")) {
                throw new IllegalArgumentException(config.substring("ERROR:".length()).trim());
            }
            AppFiles.writeConfig(this, config);
            Toast.makeText(this, "Профиль импортирован", Toast.LENGTH_SHORT).show();
            refreshState();
        } catch (Exception error) {
            new AlertDialog.Builder(this)
                    .setTitle("Ошибка профиля")
                    .setMessage(error.getMessage())
                    .setPositiveButton("OK", null)
                    .show();
        }
    }

    private void refreshState() {
        boolean running = Mobile.running();
        stateText.setText(running ? "Подключено" : "Отключено");
        stateText.setTextColor(color(running ? R.color.success : R.color.textPrimary));
        powerButton.setBackground(circle(color(running ? R.color.success : R.color.accent)));
        serverText.setText(AppFiles.serverLabel(this));
        detailText.setText(running
                ? "SOCKS5 127.0.0.1:1080 • сервис активен"
                : "SOCKS5 127.0.0.1:1080");
    }

    private void showDiagnostics() {
        if (!AppFiles.hasConfig(this)) {
            Toast.makeText(this, "Профиль ещё не импортирован", Toast.LENGTH_SHORT).show();
            return;
        }
        try {
            String config = AppFiles.readConfig(this);
            String error = Mobile.validateConfig(config);
            JSONObject json = new JSONObject(config);
            String server = json.optString("quic_server", json.optString("tls_server", "—"));
            String mode = json.optString("mode", "smart");
            String message = error.isEmpty()
                    ? "Конфигурация: OK\nСервер: " + server
                    + "\nРежим: " + mode
                    + "\nProtocol: " + Mobile.version()
                    + "\nSOCKS5: 127.0.0.1:1080"
                    : "Ошибка: " + error;
            new AlertDialog.Builder(this)
                    .setTitle("Chameleon doctor")
                    .setMessage(message)
                    .setPositiveButton("OK", null)
                    .show();
        } catch (Exception error) {
            Toast.makeText(this, error.getMessage(), Toast.LENGTH_LONG).show();
        }
    }

    private void checkUpdates(boolean userRequested) {
        boolean automatic = getPreferences(MODE_PRIVATE).getBoolean("auto_update", true);
        if (!automatic && !userRequested) return;

        UpdateRepository.fetch(this, new UpdateRepository.Callback() {
            @Override
            public void onLoaded(UpdateRepository.Index index) {
                UpdateRepository.Release latest = index.latestRelease();
                if (latest == null || !UpdateRepository.isNewer(latest.version, BuildConfig.VERSION_NAME)) {
                    if (userRequested) Toast.makeText(MainActivity.this, "Установлена актуальная версия", Toast.LENGTH_SHORT).show();
                    return;
                }

                if (automatic && latest.hasApk()) {
                    long id = UpdateRepository.download(MainActivity.this, latest);
                    if (id > 0) {
                        Toast.makeText(MainActivity.this, "Обновление " + latest.version + " скачивается", Toast.LENGTH_LONG).show();
                        return;
                    }
                }

                new AlertDialog.Builder(MainActivity.this)
                        .setTitle("Доступна версия " + latest.version)
                        .setMessage(latest.changeText())
                        .setNegativeButton("Позже", null)
                        .setPositiveButton("Открыть версии", (dialog, which) ->
                                startActivity(new Intent(MainActivity.this, VersionActivity.class)))
                        .show();
            }

            @Override
            public void onError(Exception error) {
                if (userRequested) {
                    Toast.makeText(MainActivity.this, "Не удалось проверить обновления: " + error.getMessage(), Toast.LENGTH_LONG).show();
                }
            }
        });
    }

    private void askNotificationPermission() {
        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, REQUEST_NOTIFICATIONS);
        }
    }

    private LinearLayout panel() {
        LinearLayout panel = new LinearLayout(this);
        panel.setOrientation(LinearLayout.VERTICAL);
        panel.setPadding(dp(18), dp(18), dp(18), dp(18));
        GradientDrawable background = new GradientDrawable();
        background.setColor(color(R.color.panel));
        background.setCornerRadius(dp(18));
        panel.setBackground(background);
        return panel;
    }

    private Button secondaryButton(String text) {
        Button button = new Button(this);
        button.setText(text);
        button.setTextColor(color(R.color.textPrimary));
        button.setTextSize(14);
        button.setAllCaps(false);
        GradientDrawable background = new GradientDrawable();
        background.setColor(color(R.color.panel));
        background.setCornerRadius(dp(14));
        button.setBackground(background);
        return button;
    }

    private TextView label(String text, int sp, int colorRes) {
        TextView view = new TextView(this);
        view.setText(text);
        view.setTextSize(sp);
        view.setTextColor(color(colorRes));
        return view;
    }

    private LinearLayout.LayoutParams wrap() {
        return new LinearLayout.LayoutParams(-2, -2);
    }

    private GradientDrawable circle(int color) {
        GradientDrawable shape = new GradientDrawable();
        shape.setShape(GradientDrawable.OVAL);
        shape.setColor(color);
        shape.setStroke(dp(8), Color.argb(45, 255, 255, 255));
        return shape;
    }

    private int color(int id) {
        if (Build.VERSION.SDK_INT >= 23) return getColor(id);
        return getResources().getColor(id);
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }
}
