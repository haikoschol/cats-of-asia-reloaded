# Projektstand – Cats of Asia Reloaded

Stand: **10.11.2026**. Einstieg für neue Chats; GitHub und den tatsächlichen
Arbeitsstand vor neuer Arbeit prüfen. Dieser Text ist keine Live-Serverabfrage.

## Ziel und verwandte Repositories

Dieses [Repository](https://github.com/haikoschol/cats-of-asia-reloaded) ist die
Go-Neuumsetzung der kleinen Katzenfoto-Webseite Cats of Asia. Der lokale
Codex-Projektname kann schlicht „catsofasia“ sein; auf GitHub ist **Reloaded** gemeint.
Das [Django-Original](https://github.com/haikoschol/cats-of-asia) dient als fachliche
Referenz. [Infra](https://github.com/haikoschol/cats-of-asia-infra) enthält OpenTofu,
Serverdienste, Backups und Deployment-Werkzeuge. Maschinenabhängige Ordnernamen
sind keine Voraussetzung für diese gemeinsamen Anleitungen.

Bei Erstellung: lokal `main`, Commit `1301510`, Merge von PR #66 (`pasta`, Analytics).
Das ist ein datierter Stand, keine Vorgabe für den nächsten Arbeitsbranch.

## Technischer Überblick

- Go-Version laut `go.mod` (derzeit 1.27.1), PostgreSQL 18 + PostGIS 3.6,
  serverseitige HTML-Templates und vorhandenes JavaScript; kein npm-Build nötig.
- `template/` und `static/` sind ins Go-Binary eingebettet. Nach Änderungen an
  HTML/CSS/JS neu bauen und starten; Browser-Neuladen allein reicht nicht.
- Foto-/Ortsdaten werden über `DATABASE_URL` schreibgeschützt gelesen.
  Benutzer/Sessions verwenden `USERS_DATABASE_URL`. Beide Verbindungen können
  dieselbe physische DB nutzen; die Verantwortlichkeiten bleiben getrennt.
- Fototabellen: `photos_location`, `photos_coordinates`, `photos_photo`,
  `photos_rawmetadata`, `photos_platform`, `photos_post`; zusätzlich `users` und
  `sessions`. PostGIS-Geography mit SRID 4326 und räumlichem Index ist vorhanden.
- Bilder kommen von Cloudflare Images, Kartenkacheln von Mapbox. Die DB enthält
  Metadaten/Referenzen, nicht die eigentlichen Bilddateien. Dafür ist auch lokal
  Internet und die vorhandene Konfiguration erforderlich.

## Bereits umgesetzt und gemergt

- Karte mit Ortsauswahl, Foto-Markern, Vorschauen, großem Bildbetrachter und
  direkten Foto-/Ortslinks. Gleiche Koordinaten können aufgefächert werden.
  Kartenausschnitt wird im Browser gespeichert. Standortabfrage und Fotoladen
  starten parallel; Interaktionen des Nutzers verhindern verspätete Kartensprünge.
- **#29:** Browser-Favoriten und `/favorites`, ohne Anmeldung. Gespeichert werden
  Foto-SHA-256-Hashes in localStorage, keine serverseitige Favoritenliste.
- **#30/#31:** öffentliche PostGIS-Umkreissuche `/api/photos/nearby` und `/nearme`.
  Die Seite sucht normalerweise zehn Fotos innerhalb 10 km. Für einen positiven
  UI-Test aus Deutschland Standort Bangkok simulieren: 13.75, 100.5.
- **#32:** Login/Logout, Sessions, geschützte Aktionen und CSRF-Schutz samt Tests.
  HTML-Schutz leitet zur Anmeldung um, geschützte APIs liefern anonym HTTP 401.
  Produktionscookies brauchen `SESSION_COOKIE_SECURE=true`.
- **#33:** geschützter GET `/api/photos/exists?sha256=...` für exakte Dateiduplikate.
- **#34:** geschütztes `/upload` mit JPEG-Auswahl, Vorschauen, SHA-256-Prüfung und
  lokaler EXIF-Auswertung. Dateien bis 10 MB; unbrauchbare Dateien einzeln melden.
  GPS-/Aufnahmezeit-/Zeitzonenprobleme werden angezeigt, nicht mit erfundenen
  Werten verdeckt. **Tatsächliches Hochladen und Metadaten-Bearbeitung fehlen noch.**
- **#48:** stapelbare Toasts oben rechts, einzeln schließbar, ohne automatisches
  Verschwinden.
- **#53:** optionale Plausible-Einbindung, lokal standardmäßig ausgeschaltet;
  produktiv bereits aktiviert und durch einen echten Firefox-Besuch im eigenen
  Reloaded-Dashboard bestätigt.

Diese Issues sind auf GitHub geschlossen. Details und bestehende Komponenten:
[README.md](README.md), [docs/upload.md](docs/upload.md) und
[docs/analytics.md](docs/analytics.md). Die Analytics-Anleitung enthält noch die
alte Aussage „Aktivierung ausstehend“; diese ist durch den erfolgten Rollout
überholt. Nicht deshalb erneut deployen oder den Dashboard-Eintrag doppelt anlegen.

## Lokale Entwicklung und Tests

Vom Repository-Verzeichnis aus:

```sh
./scripts/run.sh                  # baut neu und startet App + Entwicklungsdienste
./scripts/run.sh create-user NAME # verdeckte Passwortabfrage, danach beendet
./scripts/run-tests.sh            # isolierte Tests; benötigt Docker und Node.js
node scripts/upload-metadata.test.cjs # nur die Browser-Metadaten-Tests
```

- App: http://localhost:3000, lokale pgconsole: http://localhost:3001.
  Strg+C beendet den Entwicklungsbetrieb. Daten bleiben im Volume `postgres18-data`.
- Die ignorierte `.env` ist pro Rechner einzurichten; nie echte Werte committen.
  Für lokales HTTP `SESSION_COOKIE_SECURE=false` bzw. unset verwenden.
- `run-tests.sh` startet eine separate temporäre DB an Port 5433, setzt **beide**
  DB-Verbindungen darauf und entfernt nur das eigene Testvolume anschließend.
  Schema/Fixtures für Tests nicht gegen Entwicklungs-/Produktions-URLs anwenden.
- Peters ThinkPad erhielt am 28.10. eine lokale Fotodatenkopie (damals 434 Fotos,
  410 Koordinaten), bestehende lokale Benutzer/Sessions blieben erhalten. Beide
  DB-URLs wurden auf dieselbe lokale Docker-DB an Port 5432 gesetzt. Diese Kopie
  aktualisiert sich nicht automatisch. Kein SSH-Tunnel/Railway-Zugang dafür nötig.
  Das ist eine historische, rechnerbezogene Einrichtung; auf Haikos Rechner nicht
  voraussetzen. Das lokale Entwicklungsvolume ist kein aktuelles Produktionsbackup.

## Produktion und Abgrenzung zu Infra

- App: https://reloaded.catsof.asia auf Server 165.227.129.207, Caddy vor
  `catsofasia.service`. Zuletzt bekannter App-Release: `coa-1301510`.
  Produktive Konfiguration `/etc/catsofasia/app.env`, Programm `/opt/catsofasia/current`.
- Native DB `catsofasia`, Cluster `18/catsofasia`, Socket `/var/run/postgresql`,
  interner Port 5433; enthält Foto- UND Go-Benutzerdaten.
  https://pgconsole.catsof.asia bietet angemeldeten Zugriff auf Fotodaten;
  damit keine Teständerungen an echten Produktionszeilen vornehmen.
- https://analytics.catsof.asia: eigene Plausible-Instanz mit separater PostgreSQL-
  und ClickHouse-DB. Reloaded hat einen eigenen Site-Eintrag, Zeitzone
  **Asia/Bangkok**. Ursprüngliche `catsof.asia`-Statistiken erhalten.
  Tracking nutzt `/js/script.js` mit `data-domain="reloaded.catsof.asia"`.
- GitHub `.github/workflows/release.yml` testet PRs und `main`; nach erfolgreichen
  main-Tests werden Linux-amd64-Releases `coa-<Commitkurzform>` veröffentlicht.
  Merge/Pull installieren oder starten **nicht automatisch** die Server-App.
  Dafür vorhandene Installationswerkzeuge und Anleitungen im Infra-Repo verwenden.
- Infra #24 / PR #28 sind abgeschlossen: einfacher Go-/pg_dump-Backup nach R2,
  Sonntag 04:00 Europe/Berlin, vier Sicherungen behalten; manuelle Wiederherstellung.
  Nur Reloaded-DB, keine Bilddateien, App-Konfiguration oder Analytics-Daten.
  Monitoring/Benachrichtigungen bleiben mit Infra #26 bewusst zurückgestellt.

## Offene Aufgaben

- [#54 – Reverse Geocoding](https://github.com/haikoschol/cats-of-asia-reloaded/issues/54).
- [#55 – vollständige Upload-Seite/Formular](https://github.com/haikoschol/cats-of-asia-reloaded/issues/55).
  Baut auf bestehenden Login-/CSRF-, Hash- und EXIF-Funktionen sowie #54 auf.
- Vor Umsetzung aktuelle Tickettexte, PRs und den Status im GitHub-Projekt
  CatsOfAsia lesen. Diese Liste ist eine Momentaufnahme, kein Arbeitsauftrag.
