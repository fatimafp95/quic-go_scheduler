#!/usr/bin/python3

import os
import pandas as pd

df = pd.read_csv('drone.csv', dtype=object)

# clean
df = df[df.Protocol == 'UDP']
df = df[df.Info.str.startswith('58409')]
df.reset_index(drop=True, inplace=True)

df.Time = df.Time.str.replace('.', '') # s -> us
df.Time = pd.to_numeric(df.Time)

Delays = pd.concat([pd.Series([0]), df.Time[:-1]], ignore_index=True)
df['Delays'] = df.Time.subtract(Delays).astype(str) + 'us'

include_header = True
df[['Delays', 'Data Length']].to_csv('drone_processed.csv', index=False, header=include_header)
# print(df)

