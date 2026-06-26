import pandas as pd


def total(values):
    """Sum a sequence of numbers via pandas (a real third-party dependency)."""
    return int(pd.Series(values).sum())
