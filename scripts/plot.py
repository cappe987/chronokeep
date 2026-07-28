# SPDX-License-Identifier: MIT
# SPDX-FileCopyrightText: 2026 Casper Andersson <casper.casan@gmail.com>
import sys

import matplotlib.pyplot as plt
import numpy as np
from matplotlib.backends.backend_pdf import PdfPages


def make_page(info, t, s) -> plt.Figure:
    AX_GRAPH = 0
    AX_TABLE = 1
    fig, ax = plt.subplots(2, height_ratios=[2, 1])
    # Attemps to fix plot width going outside page
    # fig, ax = plt.subplots(2, height_ratios=[2, 1], tight_layout={"pad": 3, "h_pad": 5})
    # fig, ax = plt.subplots(2, height_ratios=[2, 1], tight_layout={"rect": (0, 0, 0.9, 0.9)})
    ax[AX_GRAPH].margins(0, 0.2)

    # Avoid formatting y-axis using scientific notation for large numbers
    ax[AX_GRAPH].ticklabel_format(useOffset=False, style="plain")

    # Add space for thousand separator (1 000 000 for 1000000)
    # Special handling is need because we don't want decimal point
    # when it's not needed.
    def formatter(x, pos):
        if x.is_integer():
            return f"{x:,.0f}".replace(",", " ")
        return f"{x}"
    ax[AX_GRAPH].yaxis.set_major_formatter(formatter)

    ax[AX_GRAPH].plot(t, s, color=info['color']) #, label="Time Error")
    # ax[0].legend()

    ax[AX_GRAPH].set(xlabel='Elapsed Time [s]', ylabel=info['ylabel'],
        title=info['title'])
    ax[AX_GRAPH].title.set_size(20)
    ax[AX_GRAPH].title.set_weight('bold')
    ax[AX_GRAPH].grid()

    # Table
    rows = ['Mean', 'Max', 'Min', 'Max-Min', 'Std. dev.', 'Messages']
    table_vals = [
        [f"{np.mean(s):.2f} ns"],
        [f"{np.max(s):.2f} ns"],
        [f"{np.min(s):.2f} ns"],
        [f"{np.max(s) - np.min(s):.2f} ns"],
        [f"{np.std(s):.2f} ns"],
        [f"{np.size(s)}"]]
    rows_bold = ["$\\bf{" + s + "}$" for s in rows]
    table = ax[AX_TABLE].table(cellText=table_vals,
                        colWidths=[.3] * 4,
                        rowLabels=rows_bold,
                        loc='upper center')
    table.scale(1.5, 1)
    table.set_fontsize(10)
    # Set cell height
    for r in range(len(table_vals)):
        for c in range(-1, len(table_vals[0])):
            cell = table[r, c]
            cell.set_height(0.1)
    ax[AX_TABLE].axis("off")
    return fig # pyright:ignore[reportReturnType]. It complains FigureBase is returned when it is Figure

texts = {
    'SYNC_TIME_ERROR': {'title': 'Sync Time Error', 'ylabel': 'Time Error [ns]', 'color': 'blue'},
    'DELAY_TIME_ERROR': {'title': 'Delay Time Error', 'ylabel': 'Time Error [ns]', 'color': 'blue'},
    'TWOWAY_TIME_ERROR': {'title': 'Twoway Time Error', 'ylabel': 'Time Error [ns]', 'color': 'blue'},
    'MEASURED_PDELAY': {'title': 'Measured Peer Delay', 'ylabel': 'Peer Delay [ns]', 'color': 'blue'},
    'FWD_ACCURACY': {'title': 'Forward Accuracy', 'ylabel': 'Time Error [ns]', 'color': 'blue'},

    'SYNC_LATENCY': {'title': 'Sync Latency', 'ylabel': 'Latency [ns]', 'color': 'green'},
    'DELAY_LATENCY': {'title': 'Delay Latency', 'ylabel': 'Latency [ns]', 'color': 'green'},
    'PDELAY_LATENCY': {'title': 'Peer Delay Latency', 'ylabel': 'Latency [ns]', 'color': 'green'},

    'SYNC_PDV': {'title': 'Sync PDV (Packet Delay Variance)', 'ylabel': 'PDV [ns]', 'color': 'red'},
}


with open(sys.argv[1], 'r') as f:
    data = f.read().split('\n\n')

pdf = PdfPages("output.pdf")
for x in data:
    lines = x.split('\n')
    measure = lines[0].strip()
    measure_data =  [a.split() for a in lines[1:] if a != '']
    arr = np.array(measure_data, dtype=np.float32)
    time=arr[:, 0]
    values=arr[:, 1]
    fig = make_page(texts[measure], time, values)
    fig.set_size_inches(8.27, 11.69)
    fig.savefig(pdf, format='pdf')

pdf.close()
# plt.show()
