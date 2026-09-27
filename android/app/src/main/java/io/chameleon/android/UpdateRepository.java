package io.chameleon.android;

import android.app.DownloadManager;
import android.content.Context;
import android.content.SharedPreferences;
import android.net.Uri;
import android.os.Environment;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.BufferedInputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.util.ArrayList;
import java.util.Collections;
import java.util.List;

/**
 * Reads the public Android release index from the Chameleon GitHub repository.
 *
 * The app never needs a GitHub token. This keeps update checks auditable and
 * allows users to inspect the same index and release notes in a browser.
 */
final class UpdateRepository {
    static final String INDEX_URL =
            "https://raw.githubusercontent.com/crakacr-alt/Chameleon-Protocol/main/android/releases/index.json";
    static final String PREFS = "updates";

    interface Callback {
        void onLoaded(Index index);
        void onError(Exception error);
    }

    static final class Release {
        final String version;
        final String protocol;
        final String date;
        final int minSdk;
        final String apkUrl;
        final String sha256;
        final List<String> changes;

        Release(String version, String protocol, String date, int minSdk,
                String apkUrl, String sha256, List<String> changes) {
            this.version = version;
            this.protocol = protocol;
            this.date = date;
            this.minSdk = minSdk;
            this.apkUrl = apkUrl;
            this.sha256 = sha256;
            this.changes = changes;
        }

        boolean hasApk() {
            return apkUrl != null && !apkUrl.trim().isEmpty();
        }

        String changeText() {
            StringBuilder out = new StringBuilder();
            for (String change : changes) {
                if (out.length() > 0) out.append('\n');
                out.append("• ").append(change);
            }
            return out.toString();
        }
    }

    static final class Index {
        final String latest;
        final List<Release> releases;

        Index(String latest, List<Release> releases) {
            this.latest = latest;
            this.releases = Collections.unmodifiableList(releases);
        }

        Release latestRelease() {
            for (Release release : releases) {
                if (release.version.equals(latest)) return release;
            }
            return releases.isEmpty() ? null : releases.get(0);
        }
    }

    private UpdateRepository() {}

    static void fetch(Context context, Callback callback) {
        new Thread(() -> {
            try {
                HttpURLConnection connection = (HttpURLConnection) new URL(INDEX_URL).openConnection();
                connection.setConnectTimeout(7000);
                connection.setReadTimeout(7000);
                connection.setRequestProperty("Accept", "application/json");
                connection.setRequestProperty("User-Agent", "Chameleon-Android/" + BuildConfig.VERSION_NAME);

                if (connection.getResponseCode() != 200) {
                    throw new IllegalStateException("GitHub HTTP " + connection.getResponseCode());
                }

                byte[] bytes;
                try (BufferedInputStream input = new BufferedInputStream(connection.getInputStream())) {
                    bytes = input.readAllBytes();
                } finally {
                    connection.disconnect();
                }

                JSONObject root = new JSONObject(new String(bytes, java.nio.charset.StandardCharsets.UTF_8));
                String latest = root.optString("latest", "");
                JSONArray array = root.getJSONArray("releases");
                ArrayList<Release> releases = new ArrayList<>();

                for (int i = 0; i < array.length(); i++) {
                    JSONObject item = array.getJSONObject(i);
                    JSONArray changeArray = item.optJSONArray("changes");
                    ArrayList<String> changes = new ArrayList<>();
                    if (changeArray != null) {
                        for (int c = 0; c < changeArray.length(); c++) {
                            changes.add(changeArray.getString(c));
                        }
                    }
                    releases.add(new Release(
                            item.getString("version"),
                            item.optString("protocol", ""),
                            item.optString("date", ""),
                            item.optInt("min_sdk", 23),
                            item.optString("apk_url", ""),
                            item.optString("sha256", ""),
                            changes
                    ));
                }

                Index index = new Index(latest, releases);
                context.getMainExecutor().execute(() -> callback.onLoaded(index));
            } catch (Exception error) {
                context.getMainExecutor().execute(() -> callback.onError(error));
            }
        }, "chameleon-update-check").start();
    }

    static long download(Context context, Release release) {
        if (release == null || !release.hasApk()) return -1;

        try {
            DownloadManager manager = (DownloadManager) context.getSystemService(Context.DOWNLOAD_SERVICE);
            String fileName = "Chameleon-Android-" + release.version + ".apk";
            DownloadManager.Request request = new DownloadManager.Request(Uri.parse(release.apkUrl))
                    .setTitle("Chameleon " + release.version)
                    .setDescription("Обновление Chameleon Android")
                    .setMimeType("application/vnd.android.package-archive")
                    .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE_NOTIFY_COMPLETED)
                    .setDestinationInExternalFilesDir(context, Environment.DIRECTORY_DOWNLOADS, fileName);

            long id = manager.enqueue(request);
            SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
            prefs.edit()
                    .putLong("pending_download_id", id)
                    .putString("pending_version", release.version)
                    .putString("pending_sha256", release.sha256)
                    .apply();
            return id;
        } catch (Exception ignored) {
            return -1;
        }
    }

    static boolean isNewer(String candidate, String current) {
        return compareVersions(candidate, current) > 0;
    }

    /**
     * Small semantic-version comparator supporting numeric components and
     * prerelease identifiers such as alpha.1 / beta.2.
     */
    static int compareVersions(String left, String right) {
        Version a = Version.parse(left);
        Version b = Version.parse(right);

        for (int i = 0; i < 3; i++) {
            int cmp = Integer.compare(a.numbers[i], b.numbers[i]);
            if (cmp != 0) return cmp;
        }
        if (a.pre.isEmpty() && b.pre.isEmpty()) return 0;
        if (a.pre.isEmpty()) return 1;
        if (b.pre.isEmpty()) return -1;

        int max = Math.max(a.pre.size(), b.pre.size());
        for (int i = 0; i < max; i++) {
            if (i >= a.pre.size()) return -1;
            if (i >= b.pre.size()) return 1;

            String x = a.pre.get(i);
            String y = b.pre.get(i);
            boolean xn = x.matches("\\d+");
            boolean yn = y.matches("\\d+");
            if (xn && yn) {
                int cmp = Integer.compare(Integer.parseInt(x), Integer.parseInt(y));
                if (cmp != 0) return cmp;
            } else if (xn != yn) {
                return xn ? -1 : 1;
            } else {
                int cmp = x.compareToIgnoreCase(y);
                if (cmp != 0) return cmp;
            }
        }
        return 0;
    }

    private static final class Version {
        final int[] numbers = new int[]{0, 0, 0};
        final List<String> pre = new ArrayList<>();

        static Version parse(String raw) {
            Version out = new Version();
            if (raw == null) return out;
            String clean = raw.trim();
            if (clean.startsWith("v")) clean = clean.substring(1);
            String[] split = clean.split("-", 2);
            String[] main = split[0].split("\\.");
            for (int i = 0; i < Math.min(3, main.length); i++) {
                try {
                    out.numbers[i] = Integer.parseInt(main[i].replaceAll("[^0-9].*$", ""));
                } catch (Exception ignored) {
                    out.numbers[i] = 0;
                }
            }
            if (split.length > 1) {
                String[] preParts = split[1].split("[\\.-]");
                Collections.addAll(out.pre, preParts);
            }
            return out;
        }
    }
}
