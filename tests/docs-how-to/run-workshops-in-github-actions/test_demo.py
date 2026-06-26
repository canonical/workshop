from demo import total


def test_total():
    assert total([1, 2, 3]) == 6


def test_total_negatives():
    assert total([10, -4, -6]) == 0
