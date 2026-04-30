#!/usr/bin/env python3
"""
Stromverbrauchsanalyse – spiegelt die Go-Datenstruktur aus diagramsOfTimeToUsage()
Datenstruktur: MonthlyData { MonthKey int, IInternat, IUzAula, LInternat,
               LGemeinschaft, PUzp2, PUzp3, PUzp4, SUzp4, SBhkw float64 }
"""

import sqlite3
import os
import math
import statistics
from dataclasses import dataclass
from typing import List, Dict
from collections import defaultdict

# ─────────────────────────────────────────────
# Datenstruktur (spiegelt Go-Struct MonthlyData)
# ─────────────────────────────────────────────
@dataclass
class MonthlyData:
    MonthKey:      int
    IInternat:     float
    IUzAula:       float
    LInternat:     float
    LGemeinschaft: float
    PUzp2:         float
    PUzp3:         float
    PUzp4:         float
    SUzp4:         float
    SBhkw:         float

    def total(self) -> float:
        return (self.IInternat + self.IUzAula + self.LInternat +
                self.LGemeinschaft + self.PUzp2 + self.PUzp3 +
                self.PUzp4 + self.SUzp4 + self.SBhkw)

SERIES_NAMES = {
    "IInternat":     "Internat",
    "IUzAula":       "UZ Aula",
    "LInternat":     "L13 Internat",
    "LGemeinschaft": "L13 Gemeinschaft",
    "PUzp2":         "UZ P002",
    "PUzp3":         "UZ P003",
    "PUzp4":         "UZ P004",
    "SUzp4":         "Sporthalle UZ P4",
    "SBhkw":         "Strom BHKW",
}

# ─────────────────────────────────────────────
# Daten laden (DB oder synthetische Demo-Daten)
# ─────────────────────────────────────────────
def load_data() -> List[MonthlyData]:
    db_path = "assets/data/datenbank.db"
    if os.path.exists(db_path):
        conn = sqlite3.connect(db_path)
        cur = conn.cursor()
        cur.execute("""
            SELECT month_key, i_internat, i_uz_aula, l_internat, l_gemeinschaft,
                   p_uzp2, p_uzp3, p_uzp4, s_uzp4, s_bhkw
            FROM electricity_usage
            ORDER BY month_key ASC
        """)
        rows = cur.fetchall()
        conn.close()
        return [MonthlyData(*row) for row in rows]
    else:
        # ── Synthetische Demo-Daten (2018-2024, ähnlich einem Schulgebäude) ──
        import random
        random.seed(42)
        data = []
        base = {
            "IInternat": 4200, "IUzAula": 1800, "LInternat": 950,
            "LGemeinschaft": 620, "PUzp2": 1100, "PUzp3": 980,
            "PUzp4": 870, "SUzp4": 530, "SBhkw": 2400,
        }
        # Leichter Gesamttrend: -0.5 % p.a.
        trend = -0.005
        for year in range(2018, 2025):
            for month in range(1, 13):
                mk = year * 100 + month
                # Saisonkurve: Winter ~+20%, Sommer ~-15%
                season = 1.0 + 0.20 * math.cos(math.pi * (month - 1) / 6)
                year_factor = (1 + trend) ** (year - 2018)
                row = MonthlyData(
                    MonthKey=mk,
                    IInternat=     round(base["IInternat"]     * season * year_factor * random.uniform(0.92, 1.08)),
                    IUzAula=       round(base["IUzAula"]       * season * year_factor * random.uniform(0.90, 1.10)),
                    LInternat=     round(base["LInternat"]     * season * year_factor * random.uniform(0.93, 1.07)),
                    LGemeinschaft= round(base["LGemeinschaft"] * season * year_factor * random.uniform(0.91, 1.09)),
                    PUzp2=         round(base["PUzp2"]         * season * year_factor * random.uniform(0.92, 1.08)),
                    PUzp3=         round(base["PUzp3"]         * season * year_factor * random.uniform(0.90, 1.10)),
                    PUzp4=         round(base["PUzp4"]         * season * year_factor * random.uniform(0.94, 1.06)),
                    SUzp4=         round(base["SUzp4"]         * season * year_factor * random.uniform(0.88, 1.12)),
                    SBhkw=         round(base["SBhkw"]         * season * year_factor * random.uniform(0.91, 1.09)),
                )
                data.append(row)
        print("  ⚠  Keine Datenbankdatei gefunden → synthetische Demo-Daten werden verwendet.\n")
        return data

# ─────────────────────────────────────────────
# Hilfsfunktionen
# ─────────────────────────────────────────────
def month_label(mk: int) -> str:
    y, m = mk // 100, mk % 100
    names = ["","Jan","Feb","Mär","Apr","Mai","Jun","Jul","Aug","Sep","Okt","Nov","Dez"]
    return f"{names[m]} {y}"

def totals(data: List[MonthlyData]) -> List[float]:
    return [d.total() for d in data]

def annual_totals(data: List[MonthlyData]) -> Dict[int, float]:
    by_year = defaultdict(float)
    for d in data:
        by_year[d.MonthKey // 100] += d.total()
    return dict(sorted(by_year.items()))

def sep(char="─", width=72):
    print(char * width)

def header(title: str):
    sep("═")
    print(f"  {title}")
    sep("═")

def subheader(title: str):
    sep()
    print(f"  {title}")
    sep()

def bar(value: float, max_val: float, width: int = 30) -> str:
    filled = int(round(value / max_val * width)) if max_val > 0 else 0
    return "█" * filled + "░" * (width - filled)

# ─────────────────────────────────────────────
# 1. Verbrauchsentwicklung im Zeitverlauf
# ─────────────────────────────────────────────
def analyse_1_trend(data: List[MonthlyData]):
    header("1. VERBRAUCHSENTWICKLUNG IM ZEITVERLAUF")
    vals = totals(data)
    n    = len(vals)

    # Linearer Trend (Least-Squares)
    xs = list(range(n))
    x_mean = sum(xs) / n
    y_mean = sum(vals) / n
    ss_xy = sum((xs[i] - x_mean) * (vals[i] - y_mean) for i in range(n))
    ss_xx = sum((xs[i] - x_mean) ** 2 for i in range(n))
    slope = ss_xy / ss_xx if ss_xx else 0
    trend_dir = "📈 Anstieg" if slope > 0 else "📉 Rückgang"

    subheader("Gesamttrend")
    print(f"  Richtung            : {trend_dir}  ({slope:+.1f} kWh/Monat)")
    print(f"  Erster Monat        : {month_label(data[0].MonthKey)}  →  {vals[0]:>8,.0f} kWh")
    print(f"  Letzter Monat       : {month_label(data[-1].MonthKey)}  →  {vals[-1]:>8,.0f} kWh")

    change_abs = vals[-1] - vals[0]
    change_pct = (change_abs / vals[0] * 100) if vals[0] else 0
    print(f"  Absolute Veränderung: {change_abs:>+8,.0f} kWh")
    print(f"  Prozentuale Änderg. : {change_pct:>+7.1f} %")

    subheader("Durchschnitt / Extremwerte")
    avg = sum(vals) / n
    min_v, max_v = min(vals), max(vals)
    min_idx, max_idx = vals.index(min_v), vals.index(max_v)
    print(f"  Ø Monatlicher Verb. : {avg:>8,.0f} kWh")
    print(f"  Minimum             : {min_v:>8,.0f} kWh  ({month_label(data[min_idx].MonthKey)})")
    print(f"  Maximum             : {max_v:>8,.0f} kWh  ({month_label(data[max_idx].MonthKey)})")

    subheader("Wendepunkte (lokale Extrema)")
    wendepunkte = []
    for i in range(1, n - 1):
        if vals[i] > vals[i-1] and vals[i] > vals[i+1]:
            wendepunkte.append((i, "Lokales Maximum", vals[i]))
        elif vals[i] < vals[i-1] and vals[i] < vals[i+1]:
            wendepunkte.append((i, "Lokales Minimum", vals[i]))

    # Nur die markantesten 8 zeigen
    wendepunkte.sort(key=lambda x: abs(x[2] - avg), reverse=True)
    for idx, typ, v in wendepunkte[:8]:
        print(f"  {typ:<20}: {month_label(data[idx].MonthKey)}  {v:>8,.0f} kWh")

    subheader("Veränderungsrate über den Gesamtzeitraum")
    years_span = (data[-1].MonthKey // 100) - (data[0].MonthKey // 100)
    annual_rate = (change_pct / years_span) if years_span else 0
    print(f"  Zeitraum            : {years_span} Jahre")
    print(f"  Ø jährl. Änderung   : {annual_rate:>+6.2f} % / Jahr")
    # CAGR
    if vals[0] > 0 and years_span > 0:
        cagr = ((vals[-1] / vals[0]) ** (1 / years_span) - 1) * 100
        print(f"  CAGR (monatsbasis)  : {cagr:>+6.2f} % / Jahr")

# ─────────────────────────────────────────────
# 2. Jährliche Veränderungen
# ─────────────────────────────────────────────
def analyse_2_yearly(data: List[MonthlyData]):
    header("2. JÄHRLICHE VERÄNDERUNGEN")
    ann = annual_totals(data)
    years = sorted(ann.keys())

    subheader("Jahresverbrauch & prozentuale Änderung zum Vorjahr")
    max_ann = max(ann.values())
    prev_val = None
    changes = []
    print(f"  {'Jahr':<6} {'Verbrauch':>10} kWh  {'Änderung':>9}   Balken")
    sep("-")
    for y in years:
        v = ann[y]
        if prev_val is not None:
            pct = (v - prev_val) / prev_val * 100
            changes.append((y, pct, v))
            arrow = "▲" if pct > 0 else "▼"
            pct_str = f"{arrow} {abs(pct):5.1f} %"
        else:
            pct_str = "   Basisjahr"
        b = bar(v, max_ann, 25)
        print(f"  {y:<6} {v:>12,.0f}   {pct_str:<12}  {b}")
        prev_val = v

    if changes:
        max_rise = max(changes, key=lambda x: x[1])
        max_drop = min(changes, key=lambda x: x[1])
        subheader("Ausreißer-Jahre")
        print(f"  Größter Anstieg : {max_rise[0]}  ({max_rise[1]:>+.1f} %,  {max_rise[2]:>10,.0f} kWh)")
        print(f"  Größter Rückgang: {max_drop[0]}  ({max_drop[1]:>+.1f} %,  {max_drop[2]:>10,.0f} kWh)")

        # Außergewöhnliche Schwankungen (> 1 StdAbw)
        pcts = [c[1] for c in changes]
        pct_mean = statistics.mean(pcts)
        pct_std  = statistics.stdev(pcts) if len(pcts) > 1 else 0
        threshold = pct_std
        outliers = [(y, p) for y, p, _ in changes if abs(p - pct_mean) > threshold]
        subheader(f"Außergewöhnliche Schwankungen  (|Δ − Ø| > {pct_std:.1f} %)")
        if outliers:
            for y, p in outliers:
                print(f"  {y}: {p:>+.1f} %")
        else:
            print("  Keine außergewöhnlichen Schwankungen festgestellt.")

# ─────────────────────────────────────────────
# 3. Statistische Analyse
# ─────────────────────────────────────────────
def analyse_3_statistics(data: List[MonthlyData]):
    header("3. STATISTISCHE ANALYSE (Monatsgesamtwerte)")
    vals = totals(data)
    n    = len(vals)

    mean   = statistics.mean(vals)
    median = statistics.median(vals)
    std    = statistics.stdev(vals) if n > 1 else 0
    var    = statistics.variance(vals) if n > 1 else 0
    cv     = (std / mean * 100) if mean else 0

    subheader("Kennzahlen")
    print(f"  n (Monate)          : {n}")
    print(f"  Durchschnitt        : {mean:>10,.0f} kWh")
    print(f"  Median              : {median:>10,.0f} kWh")
    print(f"  Standardabweichung  : {std:>10,.0f} kWh")
    print(f"  Varianz             : {var:>14,.0f} kWh²")
    print(f"  Variationskoeffizient: {cv:>8.1f} %")

    stability = "sehr stabil" if cv < 10 else "stabil" if cv < 20 else "moderat" if cv < 30 else "volatil"
    print(f"\n  Stabilitätsbewertung: {stability}  (CV = {cv:.1f} %)")
    print(f"  Interpretation      : {'Verbrauch schwankt wenig' if cv < 20 else 'Verbrauch zeigt deutliche Schwankungen'}")

    subheader("Quantile")
    sorted_vals = sorted(vals)
    q1   = sorted_vals[n // 4]
    q3   = sorted_vals[3 * n // 4]
    iqr  = q3 - q1
    print(f"  Q1  (25. Perz.)     : {q1:>10,.0f} kWh")
    print(f"  Q3  (75. Perz.)     : {q3:>10,.0f} kWh")
    print(f"  IQR                 : {iqr:>10,.0f} kWh")

    # Outlier nach Tukey
    lower = q1 - 1.5 * iqr
    upper = q3 + 1.5 * iqr
    outliers = [(data[i], vals[i]) for i in range(n)
                if vals[i] < lower or vals[i] > upper]
    subheader(f"Ausreißer (Tukey-Methode, Grenzen: {lower:,.0f} – {upper:,.0f} kWh)")
    if outliers:
        for d, v in outliers:
            print(f"  {month_label(d.MonthKey)}: {v:>10,.0f} kWh")
    else:
        print("  Keine Ausreißer festgestellt.")

    subheader("Analyse je Zähler")
    print(f"  {'Zähler':<22} {'Ø (kWh)':>10}  {'Std':>9}  {'CV %':>7}  {'Min':>8}  {'Max':>8}")
    sep("-")
    fields = [
        ("IInternat","IInternat"), ("IUzAula","IUzAula"),
        ("LInternat","LInternat"), ("LGemeinschaft","LGemeinschaft"),
        ("PUzp2","PUzp2"), ("PUzp3","PUzp3"), ("PUzp4","PUzp4"),
        ("SUzp4","SUzp4"), ("SBhkw","SBhkw"),
    ]
    for attr, key in fields:
        vs = [getattr(d, attr) for d in data]
        m  = statistics.mean(vs)
        s  = statistics.stdev(vs) if len(vs) > 1 else 0
        cv_s = s / m * 100 if m else 0
        print(f"  {SERIES_NAMES[key]:<22} {m:>10,.0f}  {s:>9,.0f}  {cv_s:>6.1f}%  {min(vs):>8,.0f}  {max(vs):>8,.0f}")

# ─────────────────────────────────────────────
# 4. Gleitende Durchschnitte
# ─────────────────────────────────────────────
def analyse_4_moving_avg(data: List[MonthlyData]):
    header("4. GLEITENDE DURCHSCHNITTE")
    ann  = annual_totals(data)
    years = sorted(ann.keys())
    vals  = [ann[y] for y in years]
    n     = len(vals)

    def moving_avg(series, window):
        result = []
        for i in range(len(series)):
            if i < window - 1:
                result.append(None)
            else:
                result.append(sum(series[i - window + 1:i + 1]) / window)
        return result

    ma3 = moving_avg(vals, 3)
    ma5 = moving_avg(vals, 5)

    subheader("Jahreswerte mit 3- und 5-Jahres-Gleitdurchschnitt")
    print(f"  {'Jahr':<6} {'Verbrauch':>12}  {'MA-3':>10}  {'MA-5':>10}")
    sep("-")
    for i, y in enumerate(years):
        m3 = f"{ma3[i]:>10,.0f}" if ma3[i] is not None else "         —"
        m5 = f"{ma5[i]:>10,.0f}" if ma5[i] is not None else "         —"
        print(f"  {y:<6} {vals[i]:>12,.0f}  {m3}  {m5}")

    subheader("Langfristiger Trend (aus MA-5)")
    valid5 = [(years[i], ma5[i]) for i in range(n) if ma5[i] is not None]
    if len(valid5) >= 2:
        trend5_change = valid5[-1][1] - valid5[0][1]
        trend5_pct    = trend5_change / valid5[0][1] * 100 if valid5[0][1] else 0
        print(f"  Erster MA-5-Wert    : {valid5[0][0]}  →  {valid5[0][1]:>10,.0f} kWh")
        print(f"  Letzter MA-5-Wert   : {valid5[-1][0]}  →  {valid5[-1][1]:>10,.0f} kWh")
        print(f"  Δ absolut           : {trend5_change:>+10,.0f} kWh")
        print(f"  Δ prozentual        : {trend5_pct:>+9.1f} %")
        direction = "↗ leichter Anstieg" if trend5_pct > 2 else "↘ leichter Rückgang" if trend5_pct < -2 else "→ stabil"
        print(f"  Trendbewertung      : {direction}")
    else:
        print("  Nicht genug Daten für MA-5-Trendanalyse.")

# ─────────────────────────────────────────────
# 5. Regressions- und Trendanalyse
# ─────────────────────────────────────────────
def analyse_5_regression(data: List[MonthlyData]):
    header("5. REGRESSIONS- UND TRENDANALYSE")

    ann   = annual_totals(data)
    years = sorted(ann.keys())
    vals  = [ann[y] for y in years]
    n     = len(vals)

    if n < 2:
        print("  Nicht genug Jahreswerte für Regression.")
        return

    x = list(range(n))
    x_mean = sum(x) / n
    y_mean = sum(vals) / n

    ss_xy = sum((x[i] - x_mean) * (vals[i] - y_mean) for i in range(n))
    ss_xx = sum((x[i] - x_mean) ** 2 for i in range(n))
    ss_yy = sum((vals[i] - y_mean) ** 2 for i in range(n))

    b1 = ss_xy / ss_xx if ss_xx else 0
    b0 = y_mean - b1 * x_mean
    r2 = (ss_xy ** 2 / (ss_xx * ss_yy)) if (ss_xx * ss_yy) > 0 else 0
    r  = math.sqrt(r2) * (1 if b1 >= 0 else -1)

    subheader("Lineare Regression  (Jahresbasis)")
    print(f"  Trendgleichung  : y = {b1:+.0f} · t  +  {b0:,.0f}")
    print(f"  (t = 0 entspricht dem Jahr {years[0]})")
    print(f"  Steigung (b₁)   : {b1:>+10,.0f} kWh / Jahr")
    print(f"  Achsenabschnitt : {b0:>10,.0f} kWh")
    print(f"  R               : {r:>10.4f}")
    print(f"  R²              : {r2:>10.4f}")

    fit_quality = "sehr gut" if r2 > 0.9 else "gut" if r2 > 0.7 else "moderat" if r2 > 0.5 else "schwach"
    print(f"  Modellanpassung : {fit_quality}  (R² = {r2:.4f})")

    subheader("Jährliche Veränderungsrate")
    pct_per_year = (b1 / y_mean * 100) if y_mean else 0
    print(f"  Absolute Rate   : {b1:>+10,.0f} kWh / Jahr")
    print(f"  Relative Rate   : {pct_per_year:>+9.2f} % / Jahr  (bez. auf Ø)")

    subheader("Regressionsgerade vs. Istwert  (Residuen)")
    print(f"  {'Jahr':<6} {'Istwert':>12}  {'Trend':>10}  {'Residuum':>10}  {'Res. %':>8}")
    sep("-")
    for i, y in enumerate(years):
        fitted   = b0 + b1 * i
        residual = vals[i] - fitted
        res_pct  = residual / fitted * 100 if fitted else 0
        print(f"  {y:<6} {vals[i]:>12,.0f}  {fitted:>10,.0f}  {residual:>+10,.0f}  {res_pct:>+7.1f} %")

    subheader("Prognose (lineare Fortschreibung)")
    last_year = years[-1]
    print(f"  {'Jahr':<6} {'Prognose':>12}")
    sep("-")
    for step in range(1, 4):
        proj_year  = last_year + step
        proj_val   = b0 + b1 * (n - 1 + step)
        print(f"  {proj_year:<6} {proj_val:>12,.0f} kWh")
    print()
    print("  ⚠  Prognose basiert auf linearer Extrapolation –")
    print("     Änderungen im Verhalten können abweichen.")

# ─────────────────────────────────────────────
# MAIN
# ─────────────────────────────────────────────
def main():
    print()
    print("╔══════════════════════════════════════════════════════════════════════╗")
    print("║          STROMVERBRAUCHSANALYSE  –  VOLLSTÄNDIGER BERICHT           ║")
    print("╚══════════════════════════════════════════════════════════════════════╝")
    print()

    data = load_data()
    data.sort(key=lambda d: d.MonthKey)

    print(f"  Datenpunkte geladen : {len(data)} Monate")
    print(f"  Zeitraum            : {month_label(data[0].MonthKey)}  →  {month_label(data[-1].MonthKey)}")
    print(f"  Zähler              : {len(SERIES_NAMES)} Messpunkte")

    analyse_1_trend(data)
    analyse_2_yearly(data)
    analyse_3_statistics(data)
    analyse_4_moving_avg(data)
    analyse_5_regression(data)

    sep("═")
    print("  Analyse abgeschlossen.")
    sep("═")
    print()

if __name__ == "__main__":
    main()