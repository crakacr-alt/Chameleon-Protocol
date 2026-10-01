package io.chameleon.android;

import android.Manifest;
import android.app.Activity;
import android.app.AlertDialog;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.net.ConnectivityManager;
import android.net.Network;
import android.net.NetworkCapabilities;
import android.net.Uri;
import android.net.VpnService;
import android.os.Build;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.view.View;
import android.widget.AdapterView;
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
 * One-button Android client for the shared Chameleon core.
 *
 * Smart, Inspector and VPN are real Android VpnService modes on Android 10+.
 * Inspector replaces the old local-only Proxy mode with a visible traffic
 * analysis workspace and PCAP capture.
 */
public final class MainActivity extends Activity {
    private static final int PICK_PROFILE = 2001;
    private static final int REQUEST_NOTIFICATIONS = 2002;
    private static final int REQUEST_VPN = 2003;

    private final Handler handler = new Handler(Looper.getMainLooper());

    private TextView stateText;
    private TextView serverText;
    private TextView detailText;
    private TextView coexistText;
    private Button powerButton;
    private Spinner modeSpinner;
    private CheckBox autoUpdate;
    private boolean modeEventsEnabled;

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
        restoreModeSelection();
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

        coexistText = label("", 13, R.color.success);
        LinearLayout.LayoutParams coexistParams = wrap();
        coexistParams.topMargin = dp(20);
        root.addView(coexistText, coexistParams);

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

        card.addView(label("СЕРВЕР", 11, R.color.textSecondary));
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
                "Smart — системный VPN: direct + Chameleon",
                "Inspector — сниффер трафика + Chameleon",
                "VPN — весь телефон только через Chameleon"
        };
        ArrayAdapter<String> adapter = new ArrayAdapter<>(
                this,
                android.R.layout.simple_spinner_dropdown_item,
                modes
        );
        modeSpinner.setAdapter(adapter);
        modeSpinner.setOnItemSelectedListener(new AdapterView.OnItemSelectedListener() {
            @Override
            public void onItemSelected(AdapterView<?> parent, View view, int position, long id) {
                if (modeEventsEnabled) applySelectedMode();
            }

            @Override
            public void onNothingSelected(AdapterView<?> parent) {
            }
        });
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

        Button inspectorButton = secondaryButton("Открыть Inspector / сниффер");
        inspectorButton.setOnClickListener(v -> startActivity(new Intent(this, InspectorActivity.class)));
        LinearLayout.LayoutParams inspectorParams = new LinearLayout.LayoutParams(-1, dp(48));
        inspectorParams.topMargin = dp(8);
        root.addView(inspectorButton, inspectorParams);

        TextView note = label(
                "Inspector записывает трафик только после явного запуска и сохраняет захват локально на устройстве.",
                12,
                R.color.textSecondary
        );
        note.setGravity(Gravity.CENTER);
        LinearLayout.LayoutParams noteParams = new LinearLayout.LayoutParams(-1, -2);
        noteParams.topMargin = dp(22);
        root.addView(note, noteParams);

        return scroll;
    }

    private void restoreModeSelection() {
        modeEventsEnabled = false;
        String mode = (!AppFiles.hasConfig(this) && Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q)
                ? "vpn"
                : AppFiles.runtimeMode(this);
        if ("proxy".equals(mode)) {
            // 1.0.x called the local-only SOCKS mode "Proxy". In 1.1 it is
            // replaced by the full-device Inspector, so migrate the UI choice.
            mode = "inspector";
            AppFiles.setRuntimeMode(this, mode);
        }
        int position = "vpn".equals(mode) ? 2 : ("inspector".equals(mode) ? 1 : 0);
        modeSpinner.setSelection(position, false);
        modeEventsEnabled = true;
        updateCompatibilityLabel();
    }

    private String selectedMode() {
        int position = modeSpinner.getSelectedItemPosition();
        if (position == 2) return "vpn";
        if (position == 1) return "inspector";
        return "smart";
    }

    private void applySelectedMode() {
        String mode = selectedMode();
        AppFiles.setRuntimeMode(this, mode);
        updateCompatibilityLabel();

        if (!AppFiles.hasConfig(this)) return;

        try {
            String coreMode = "smart".equals(mode) ? "smart" : "proxy";
            AppFiles.setCoreMode(this, coreMode);

            if (ChameleonVpnService.running()) {
                ChameleonVpnService.requestStop(this);
                Toast.makeText(this, "VPN остановлен. Нажмите подключение снова.", Toast.LENGTH_SHORT).show();
            } else if (Mobile.running()) {
                Intent stop = new Intent(this, ChameleonService.class);
                stop.setAction(ChameleonService.ACTION_STOP);
                startService(stop);
                Toast.makeText(this, "Режим изменён. Нажмите подключение снова.", Toast.LENGTH_SHORT).show();
            }
        } catch (Exception error) {
            Toast.makeText(this, "Не удалось сохранить режим: " + error.getMessage(), Toast.LENGTH_LONG).show();
        }
    }

    private void updateCompatibilityLabel() {
        int position = modeSpinner == null ? 0 : modeSpinner.getSelectedItemPosition();
        boolean smart = position == 0;
        String label;
        if (smart) {
            label = "● Smart VPN • direct + Chameleon";
        } else if (position == 1) {
            label = "● Inspector • захват трафика через системный VPN";
        } else {
            label = "● VPN • весь трафик только через Chameleon";
        }
        coexistText.setText(label);
        coexistText.setTextColor(color(R.color.textSecondary));
    }

    private void toggleConnection() {
        if (ChameleonVpnService.running() || ChameleonVpnService.starting()) {
            ChameleonVpnService.requestStop(this);
            return;
        }

        if (ChameleonService.RUNTIME_OWNER.equals(Mobile.owner())) {
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

        String mode = selectedMode();
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.Q) {
            Toast.makeText(
                    this,
                    "Smart, Inspector и VPN требуют Android 10 или новее",
                    Toast.LENGTH_LONG
            ).show();
            return;
        }

        AppFiles.setRuntimeMode(this, mode);
        Intent permission = VpnService.prepare(this);
        if (permission != null) {
            startActivityForResult(permission, REQUEST_VPN);
        } else {
            startVpn();
        }
    }

    private void startVpn() {
        String mode = selectedMode();
        try {
            AppFiles.setRuntimeMode(this, mode);
            AppFiles.setCoreMode(this, "smart".equals(mode) ? "smart" : "proxy");
        } catch (Exception error) {
            Toast.makeText(this, "Ошибка режима: " + error.getMessage(), Toast.LENGTH_LONG).show();
            return;
        }

        Intent start = new Intent(this, ChameleonVpnService.class);
        start.setAction(ChameleonVpnService.ACTION_START);
        start.putExtra(ChameleonVpnService.EXTRA_MODE, mode);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            startForegroundService(start);
        } else {
            startService(start);
        }

        if ("inspector".equals(mode)) {
            startActivity(new Intent(this, InspectorActivity.class));
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

        if (requestCode == REQUEST_VPN) {
            if (resultCode == RESULT_OK) {
                startVpn();
            } else {
                Toast.makeText(this, "Разрешение VPN не выдано", Toast.LENGTH_SHORT).show();
            }
            return;
        }

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

            String profile = new String(bytes.toByteArray(), StandardCharsets.UTF_8);
            String mode = selectedMode();
            String coreMode = "smart".equals(mode) ? "smart" : "proxy";
            String stateDir = new java.io.File(getFilesDir(), "state").getAbsolutePath();
            String config = Mobile.buildConfig(profile, stateDir, coreMode);
            if (config.startsWith("ERROR:")) {
                throw new IllegalArgumentException(config.substring("ERROR:".length()).trim());
            }

            AppFiles.writeConfig(this, config);
            AppFiles.setRuntimeMode(this, mode);
            AppFiles.clearLastVpnError(this);
            refreshState();

            new AlertDialog.Builder(this)
                    .setTitle("Профиль готов")
                    .setMessage("Профиль сохранён. Проверка ingress и Auth v2 будет выполнена после запуска Chameleon через прямой Wi-Fi/мобильный канал, чтобы активный Happ/V2Ray/другой VPN не влиял на результат.")
                    .setPositiveButton("Подключить", (dialog, which) -> toggleConnection())
                    .setNegativeButton("Позже", null)
                    .show();
        } catch (Exception error) {
            new AlertDialog.Builder(this)
                    .setTitle("Ошибка профиля")
                    .setMessage(error.getMessage())
                    .setPositiveButton("OK", null)
                    .show();
        }
    }

    private void refreshState() {
        boolean vpn = ChameleonVpnService.running();
        boolean vpnStarting = ChameleonVpnService.starting();
        boolean sidecar = ChameleonService.RUNTIME_OWNER.equals(Mobile.owner())
                && Mobile.listenerReady() && !vpn;
        boolean running = vpn || vpnStarting || sidecar;

        String persistedError = AppFiles.lastVpnError(this);
        boolean failed = !running && !persistedError.isEmpty();
        stateText.setText(vpn
                ? "VPN подключён"
                : (vpnStarting ? "VPN подключается…"
                : (sidecar ? "Подключено" : (failed ? "Ошибка подключения" : "Отключено"))));
        stateText.setTextColor(color(running ? R.color.success : (failed ? R.color.danger : R.color.textPrimary)));
        powerButton.setBackground(circle(color(running ? R.color.success : R.color.accent)));
        serverText.setText(AppFiles.serverLabel(this));
        updateCompatibilityLabel();

        String mode = AppFiles.runtimeMode(this);
        if (vpn) {
            detailText.setText("VPN/TUN • весь телефон • SOCKS5 127.0.0.1:1080");
        } else if (vpnStarting) {
            detailText.setText("Проверка сервера → авторизация → SOCKS5 → TUN…");
        } else if (sidecar) {
            detailText.setText("SOCKS5 127.0.0.1:1080 • " + mode.toUpperCase() + " активен");
        } else if (failed) {
            detailText.setText(persistedError);
        } else {
            detailText.setText("Готов к подключению");
        }
    }

    private boolean hasExternalVpnTransport() {
        if (ChameleonVpnService.running() || ChameleonVpnService.starting()) {
            return false;
        }

        ConnectivityManager manager =
                (ConnectivityManager) getSystemService(CONNECTIVITY_SERVICE);
        if (manager == null) return false;

        for (Network network : manager.getAllNetworks()) {
            NetworkCapabilities caps = manager.getNetworkCapabilities(network);
            if (caps != null && caps.hasTransport(NetworkCapabilities.TRANSPORT_VPN)) {
                return true;
            }
        }
        return false;
    }

    private void showDiagnostics() {
        if (!AppFiles.hasConfig(this)) {
            Toast.makeText(this, "Профиль ещё не импортирован", Toast.LENGTH_SHORT).show();
            return;
        }

        final String config;
        try {
            config = AppFiles.readConfig(this);
        } catch (Exception error) {
            Toast.makeText(this, error.getMessage(), Toast.LENGTH_LONG).show();
            return;
        }

        Toast.makeText(this, "Проверяю реальное подключение…", Toast.LENGTH_SHORT).show();

        new Thread(() -> {
            try {
                String error = Mobile.validateConfig(config);
                JSONObject json = new JSONObject(config);
                String server = json.optString("tls_server",
                        json.optString("quic_server",
                                json.optString("tcp_server", "—")));
                String mode = json.optString("mode", "smart");
                boolean listenerReady = Mobile.listenerReady();
                String listener = listenerReady
                        ? "active"
                        : (Mobile.running() ? "broken" : "stopped");
                String vpnState = ChameleonVpnService.running()
                        ? "active"
                        : (ChameleonVpnService.starting() ? "starting" : "stopped");
                String lastError = Mobile.lastError();
                if (lastError.isEmpty()) lastError = AppFiles.lastVpnError(this);

                boolean otherVpn = hasExternalVpnTransport();
                String remoteStatus;
                if (error.isEmpty() && otherVpn
                        && !ChameleonVpnService.running()
                        && !ChameleonVpnService.starting()) {
                    remoteStatus = "ОТЛОЖЕНО • активен другой VPN; ingress будет проверен напрямую после запуска Chameleon";
                } else if (error.isEmpty()) {
                    String prepared = Mobile.prepareVPNConfig(config);
                    if (prepared.startsWith("ERROR:")) {
                        remoteStatus = "FAIL • " + prepared.substring("ERROR:".length()).trim();
                    } else {
                        JSONObject selected = new JSONObject(prepared);
                        String tcpTransport = selected.optString("tcp_transport", "auto");
                        String tcpEndpoint = selected.optString("tls_server",
                                selected.optString("quic_server",
                                        selected.optString("tcp_server", "—")));
                        String udpMode = selected.optString("udp_mode", "auto");
                        String udpStatus = "quic".equalsIgnoreCase(udpMode)
                                ? "QUIC"
                                : "DNS-over-TCP fallback";
                        remoteStatus = "OK • TCP " + tcpTransport.toUpperCase()
                                + " " + tcpEndpoint + " • UDP " + udpStatus;
                    }
                } else {
                    remoteStatus = "не проверялось";
                }

                String message = error.isEmpty()
                        ? "Конфигурация: OK\nСервер профиля: " + server
                        + "\nУдалённый транспорт: " + remoteStatus
                        + "\nРежим: " + mode
                        + "\nProtocol: " + Mobile.version()
                        + "\nSOCKS5: 127.0.0.1:1080"
                        + "\nListener: " + listener
                        + "\nOwner: " + (Mobile.owner().isEmpty() ? "—" : Mobile.owner())
                        + "\nChameleon VPN: " + vpnState
                        + "\nДругой VPN: " + (otherVpn ? "active" : "не обнаружен")
                        + (lastError.isEmpty() ? "" : "\nLast error: " + lastError)
                        : "Ошибка: " + error;

                runOnUiThread(() -> new AlertDialog.Builder(this)
                        .setTitle("Chameleon doctor")
                        .setMessage(message)
                        .setPositiveButton("OK", null)
                        .show());
            } catch (Exception diagnosticError) {
                runOnUiThread(() -> Toast.makeText(
                        this,
                        diagnosticError.getMessage(),
                        Toast.LENGTH_LONG
                ).show());
            }
        }, "chameleon-doctor").start();
    }

    private void checkUpdates(boolean userRequested) {
        boolean automatic = getPreferences(MODE_PRIVATE).getBoolean("auto_update", true);
        if (!automatic && !userRequested) return;

        UpdateRepository.fetch(this, new UpdateRepository.Callback() {
            @Override
            public void onLoaded(UpdateRepository.Index index) {
                UpdateRepository.Release latest = index.latestRelease();
                if (latest == null || !UpdateRepository.isNewer(latest.version, BuildConfig.VERSION_NAME)) {
                    if (userRequested) {
                        Toast.makeText(MainActivity.this, "Установлена актуальная версия", Toast.LENGTH_SHORT).show();
                    }
                    return;
                }

                if (automatic && latest.hasApk()) {
                    long id = UpdateRepository.download(MainActivity.this, latest);
                    if (id > 0) {
                        Toast.makeText(
                                MainActivity.this,
                                "Обновление " + latest.version + " скачивается",
                                Toast.LENGTH_LONG
                        ).show();
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
                    Toast.makeText(
                            MainActivity.this,
                            "Не удалось проверить обновления: " + error.getMessage(),
                            Toast.LENGTH_LONG
                    ).show();
                }
            }
        });
    }

    private void askNotificationPermission() {
        if (Build.VERSION.SDK_INT >= 33
                && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS)
                != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(
                    new String[]{Manifest.permission.POST_NOTIFICATIONS},
                    REQUEST_NOTIFICATIONS
            );
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

    private GradientDrawable circle(int value) {
        GradientDrawable shape = new GradientDrawable();
        shape.setShape(GradientDrawable.OVAL);
        shape.setColor(value);
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
