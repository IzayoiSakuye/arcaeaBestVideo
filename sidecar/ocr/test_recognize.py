"""判定行模型输出的基本解码约束。"""

import unittest

import numpy as np
from PIL import Image

from recognize import decode, prepare


class RecognizeTest(unittest.TestCase):
    def test_ctc_decode_and_resize(self):
        logits = np.eye(3, dtype=np.float32)[[0, 1, 1, 0, 2]][None]
        self.assertEqual(decode(logits, ["blank", "A", "B"]), [{"text": "AB"}])

        image = Image.new("RGB", (192, 14), "white")
        pixels = prepare(image, (0, 0, 192, 14))
        self.assertEqual(pixels.shape, (3, 48, 659))
        self.assertTrue(np.allclose(pixels, 1))


if __name__ == "__main__":
    unittest.main()
