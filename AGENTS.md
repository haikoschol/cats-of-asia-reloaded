# Arbeitsregeln für Cats of Asia Reloaded

## Einstieg in jeden Chat

1. Lies zuerst [PROJEKTSTAND.md](PROJEKTSTAND.md).
2. Prüfe Branch und `git status`. Wechsle oder bereinige nichts ungefragt.
3. Lies das betroffene GitHub-Issue und vorhandene PR-Kommentare, wenn der Auftrag
   darauf verweist. Den aktuellen Status im Projekt CatsOfAsia prüfen, wenn die
   nächste Aufgabe noch geklärt werden soll; nicht selbst ein Ticket anfangen.

## Maßstab und Zusammenarbeit

- Kleine Katzenfoto-Webseite: die einfachste Lösung für den konkreten Auftrag.
  Keine zusätzlichen Systeme, Abstraktionen oder Funktionen auf Vorrat.
- Fotodaten sind nicht hochkritisch; Ausfallzeit bei manueller Wiederherstellung
  ist akzeptabel. Zugangsdaten und Benutzerkonten trotzdem schützen.
- Go und die vorhandene Architektur bevorzugen. Bestehendes JavaScript für die
  Browseroberfläche weiterverwenden; keine neue Sprache, Build-Kette oder größere
  Architekturänderung stillschweigend einführen.
- Antworte in der Sprache des Nutzers, verständlich und ohne unnötigen Fachjargon.
  Bei Peter auf Deutsch erklären. README-Dateien kurz und praktisch halten.
- Bei „nur erklären/nachschauen“ keine Dateien oder Dienste verändern. Klare
  Umsetzungsaufträge innerhalb ihres Umfangs selbstständig erledigen.
- Commit, Push, Merge und GitHub-Nachrichten nur auf ausdrücklichen Auftrag.
  Bereits erteilte Zustimmung nicht für jeden Teilschritt erneut abfragen.

## Entwickeln und prüfen

- Vorhandene Login-/CSRF-, Hash- und EXIF-Funktionen wiederverwenden. Fehlende
  Foto-Koordinaten, Aufnahmezeiten oder Zeitzonen nicht durch Vermutungen ersetzen.
- Für relevante Codeänderungen `./scripts/run-tests.sh` verwenden; es isoliert
  beide DB-Verbindungen in einer temporären Testdatenbank. EXIF-Tests allein:
  `node scripts/upload-metadata.test.cjs`. Tests passend zur Änderung wählen;
  reine Dokumentation braucht keinen App-/Datenbanklauf.
- Keine Tests/Migrationen gegen Produktion oder die Originaldatenbank ausführen.
  Secrets, komplette DB-URLs und private Konfiguration weder ausgeben noch committen.
- Eigene App-/Testläufe anschließend beenden. Das Entwicklungsvolume
  `postgres18-data` und vorhandene lokale Daten erhalten; kein ungefragtes
  `docker compose down --volumes` für die Entwicklungsumgebung.
- Templates und Assets sind eingebettet: nach Änderungen neu bauen und starten.
  Produktive Deployments nur im beauftragten Umfang, über die Infra-Werkzeuge.

## Projektwissen pflegen

Aktualisiere `PROJEKTSTAND.md` bei wesentlichen abgeschlossenen Änderungen oder
neuen Entscheidungen samt Datum. Halte es kompakt, ohne Chatprotokolle/Secrets.
Unterscheide geplant, implementiert, gemergt, ausgerollt und tatsächlich geprüft;
historische Prüfungen sind keine Live-Garantie.
