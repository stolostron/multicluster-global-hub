import matplotlib.pyplot as plt
from datetime import datetime
import matplotlib.dates as mdates
from mpl_toolkits.axes_grid1 import host_subplot
import pandas as pd
import os
from matplotlib.ticker import MaxNLocator

script_path = os.path.realpath(__file__)
output_path = os.path.join(os.path.dirname(script_path), "../output")
if not os.path.exists(output_path):
    os.makedirs(output_path)
    
# Initialization
plt.clf()

df = pd.read_csv(output_path + '/count-initialization.csv')
print(df)
dates = [datetime.strptime(d, "%Y-%m-%d %H:%M:%S") for d in df['time']]

host = host_subplot(111)
twin = host.twinx()

# time,compliance,event,cluster,heartbeats
p1, = host.plot_date(dates, df['compliance'], '*-', alpha=0.8, label="Compliance")
p2, = host.plot_date(dates, df['event'], '>--', color=p1.get_color(), alpha=0.8, label="Event")
p3, = twin.plot_date(dates, df['heartbeats'], 'o-', color='green', alpha=0.8, label="Cluster")

host.legend(labelcolor="linecolor")

host.set_ylabel("Compliance and Event")
twin.set_ylabel("Cluster")

host.yaxis.get_label().set_color(p1.get_color())
twin.yaxis.get_label().set_color(p3.get_color())

host.set_xlabel("Time")
host.xaxis.set_major_formatter(mdates.DateFormatter("%M:%S"))
host.xaxis.set_major_locator(mdates.SecondLocator(interval=20))

host.yaxis.set_major_locator(MaxNLocator(integer=True))
twin.yaxis.set_major_locator(MaxNLocator(integer=True))

plt.grid(axis='y', color='0.95')

plt.title("Global Hub Resources Counter")
plt.savefig(output_path + '/count-initialization.png')


# compliance
plt.clf()
# time,compliant,non_compliant
df = pd.read_csv(output_path + '/count-compliance.csv')
dates = [datetime.strptime(d, "%Y-%m-%d %H:%M:%S") for d in df['time']]

fig, ax = plt.subplots()
ax.plot(dates, df['compliant'], 'o-', color="green", label="Compliant")
ax.plot(dates, df['non_compliant'], '*-', color="red", label="NonCompliant")

ax.legend(labelcolor="linecolor")

ax.set_xlabel("Time")
ax.xaxis.set_major_formatter(mdates.DateFormatter("%M:%S"))
ax.xaxis.set_major_locator(mdates.SecondLocator(interval=20))

ax.yaxis.set_major_locator(MaxNLocator(integer=True))

plt.title("Global Hub Compliance Counter")
plt.savefig(output_path + '/count-compliance.png')




# COLOR_TEMPERATURE = "#69b3a2"
# COLOR_PRICE = "#3399e6"

# fig, ax1 = plt.subplots()
# ax2 = ax1.twinx()

# ax1.plot(df.loc[df['Name']=='compliance', 'Time'], df.loc[df['Name']=='compliance', 'Count'], color=COLOR_TEMPERATURE, lw=3)
# ax2.plot(df.loc[df['Name']=='compliance', 'Time'], price, color=COLOR_PRICE, lw=4)

# ax1.set_xlabel("Date")
# ax1.set_ylabel("Temperature (Celsius °)", color=COLOR_TEMPERATURE, fontsize=14)
# ax1.tick_params(axis="y", labelcolor=COLOR_TEMPERATURE)

# ax2.set_ylabel("Price ($)", color=COLOR_PRICE, fontsize=14)
# ax2.tick_params(axis="y", labelcolor=COLOR_PRICE)

# fig.suptitle("Temperature down, price up", fontsize=20)
# fig.autofmt_xdate()

# host = host_subplot(111)
# twin = host.twinx()

# host.set_xlabel("Time")

# p1, = host.plot_date(df["Time"], compliance, '-', alpha=0.8, label="compliance")
# p2, = host.plot_date(dates, event, '--', color=p1.get_color(), alpha=0.8, label="event")
# p3, = twin.plot_date(dates, cluster, 'o-', color='green', alpha=0.8, label="cluster")

# host.legend(labelcolor="linecolor")

# host.set_ylabel("Compliance and Event")
# twin.set_ylabel("Cluster")

# host.yaxis.get_label().set_color(p1.get_color())
# twin.yaxis.get_label().set_color(p3.get_color())

# plt.grid(axis='y', color='0.95')

# lines=plt.plot(df['Time'], , years, death)
# plt.grid(True)
# plt.setp(lines,color=(1,.4,.5),marker='*')
# plt.show()

# plt.savefig(output_path + '/stopwatch-initialization.png')

# Display the DataFrame
# df = pd.read_csv(output_path + '/stopwatch-compliance.csv', header=None, names=["Time", "Status", "Count"])
# print(df)
