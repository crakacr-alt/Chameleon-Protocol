package io.chameleon.android;

import android.app.Activity;
import android.app.AlertDialog;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
import android.os.Build;
import android.os.Bundle;
import android.view.Gravity;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.ProgressBar;
import android.widget.ScrollView;
import android.widget.TextView;
import android.widget.Toast;

/**
 * Human-readable release browser.
 *
 * Versions and notes come from android/releases/index.json in the same public
 * repository, so the UI never invents or hides release history.
 */
public final class VersionActivity extends Activity {
    private LinearLayout list;
    private ProgressBar progress;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        setContentView(buildUi());
        load();
    }

    private ScrollView buildUi() {
        ScrollView scroll = new ScrollView(this);
        scroll.setBackgroundColor(color(R.color.bg));

        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setPadding(dp(20), dp(24), dp(20), dp(30));
        scroll.addView(root, new ScrollView.LayoutParams(-1, -2));

        TextView title = label("Версии Chameleon", 23, R.color.textPrimary);
        title.setTypeface(Typeface.DEFAULT_BOLD);
        root.addView(title);

        TextView current = label(
                "Установлено: " + BuildConfig.VERSION_NAME,
                13,
                R.color.textSecondary
        );
        LinearLayout.LayoutParams currentParams = wrap();
        currentParams.topMargin = dp(5);
        root.addView(current, currentParams);

        TextView hint = label(
                "APK берутся только из официальных GitHub releases Chameleon. Android всегда показывает системное подтверждение установки.",
                12,
                R.color.textSecondary
        );
        LinearLayout.LayoutParams hintParams = new LinearLayout.LayoutParams(-1, -2);
        hintParams.topMargin = dp(14);
        root.addView(hint, hintParams);

        progress = new ProgressBar(this);
        LinearLayout.LayoutParams progressParams = wrap();
        progressParams.gravity = Gravity.CENTER_HORIZONTAL;
        progressParams.topMargin = dp(26);
        root.addView(progress, progressParams);

        list = new LinearLayout(this);
        list.setOrientation(LinearLayout.VERTICAL);
        LinearLayout.LayoutParams listParams = new LinearLayout.LayoutParams(-1, -2);
        listParams.topMargin = dp(12);
        root.addView(list, listParams);

        return scroll;
    }

    private void load() {
        UpdateRepository.fetch(this, new UpdateRepository.Callback() {
            @Override
            public void onLoaded(UpdateRepository.Index index) {
                progress.setVisibility(android.view.View.GONE);
                list.removeAllViews();
                if (index.releases.isEmpty()) {
                    list.addView(label("Пока нет опубликованных Android-релизов.", 14, R.color.textSecondary));
                    return;
                }
                for (UpdateRepository.Release release : index.releases) {
                    list.addView(releaseCard(release));
                }
            }

            @Override
            public void onError(Exception error) {
                progress.setVisibility(android.view.View.GONE);
                new AlertDialog.Builder(VersionActivity.this)
                        .setTitle("Не удалось получить версии")
                        .setMessage(error.getMessage())
                        .setPositiveButton("Повторить", (dialog, which) -> {
                            progress.setVisibility(android.view.View.VISIBLE);
                            load();
                        })
                        .setNegativeButton("Закрыть", null)
                        .show();
            }
        });
    }

    private LinearLayout releaseCard(UpdateRepository.Release release) {
        LinearLayout card = new LinearLayout(this);
        card.setOrientation(LinearLayout.VERTICAL);
        card.setPadding(dp(17), dp(15), dp(17), dp(15));

        GradientDrawable bg = new GradientDrawable();
        bg.setColor(color(R.color.panel));
        bg.setCornerRadius(dp(16));
        card.setBackground(bg);

        boolean current = release.version.equals(BuildConfig.VERSION_NAME);
        TextView name = label(
                release.version + (current ? "  •  установлена" : ""),
                18,
                current ? R.color.success : R.color.textPrimary
        );
        name.setTypeface(Typeface.DEFAULT_BOLD);
        card.addView(name);

        TextView meta = label(
                "Protocol " + release.protocol
                        + "  •  " + release.date
                        + "  •  Android " + sdkName(release.minSdk) + "+",
                11,
                R.color.textSecondary
        );
        LinearLayout.LayoutParams metaParams = wrap();
        metaParams.topMargin = dp(4);
        card.addView(meta, metaParams);

        TextView changes = label(release.changeText(), 13, R.color.textPrimary);
        LinearLayout.LayoutParams changesParams = new LinearLayout.LayoutParams(-1, -2);
        changesParams.topMargin = dp(12);
        card.addView(changes, changesParams);

        Button action = new Button(this);
        action.setAllCaps(false);
        action.setTextColor(color(R.color.textPrimary));
        action.setTextSize(13);

        if (current) {
            action.setText("Текущая версия");
            action.setEnabled(false);
        } else if (!release.hasApk()) {
            action.setText("APK ещё не опубликован");
            action.setEnabled(false);
        } else {
            boolean older = UpdateRepository.compareVersions(release.version, BuildConfig.VERSION_NAME) < 0;
            action.setText(older ? "Скачать старую версию" : "Скачать и установить");
            action.setOnClickListener(v -> {
                if (Build.VERSION.SDK_INT < release.minSdk) {
                    Toast.makeText(this, "Эта версия требует более новый Android", Toast.LENGTH_LONG).show();
                    return;
                }
                long id = UpdateRepository.download(this, release);
                if (id <= 0) {
                    Toast.makeText(this, "Не удалось начать загрузку", Toast.LENGTH_LONG).show();
                } else {
                    Toast.makeText(this, "APK скачивается. После загрузки появится уведомление.", Toast.LENGTH_LONG).show();
                }
            });
        }

        LinearLayout.LayoutParams actionParams = new LinearLayout.LayoutParams(-1, dp(48));
        actionParams.topMargin = dp(14);
        card.addView(action, actionParams);

        LinearLayout.LayoutParams cardParams = new LinearLayout.LayoutParams(-1, -2);
        cardParams.bottomMargin = dp(12);
        card.setLayoutParams(cardParams);
        return card;
    }

    private String sdkName(int minSdk) {
        switch (minSdk) {
            case 23: return "6.0";
            case 24: return "7.0";
            case 26: return "8.0";
            case 28: return "9";
            case 29: return "10";
            case 30: return "11";
            case 31: return "12";
            case 33: return "13";
            case 34: return "14";
            case 35: return "15";
            default: return "API " + minSdk;
        }
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

    private int color(int id) {
        if (Build.VERSION.SDK_INT >= 23) return getColor(id);
        return getResources().getColor(id);
    }

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }
}
