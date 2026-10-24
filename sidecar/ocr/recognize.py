"""用本地 PP-OCRv6 small 模型批量读取 B50 文字。"""

import json
import math
import sys
from pathlib import Path

import numpy as np
import onnxruntime as ort
import yaml
from PIL import Image


def decode(prediction, characters):
    rows = []
    for ids in prediction.argmax(axis=-1):
        previous = -1
        text = []
        for index in ids:
            if index != 0 and index != previous:
                text.append(characters[index])
            previous = index
        rows.append({"text": "".join(text)})
    return rows


def prepare(sheet, box):
    row = sheet.crop(box)
    width = math.ceil(48 * row.width / row.height)
    pixels = np.asarray(row.resize((width, 48), Image.Resampling.BILINEAR), dtype=np.float32)
    return ((pixels.transpose(2, 0, 1) / 255 - 0.5) / 0.5).astype(np.float32)


def main():
    model_dir, output_path = map(Path, sys.argv[1:3])
    request = json.load(sys.stdin)
    config = yaml.safe_load((model_dir / "inference.yml").read_text(encoding="utf-8"))
    characters = ["blank"] + config["PostProcess"]["character_dict"] + [" "]
    session = ort.InferenceSession(
        str(model_dir / "inference.onnx"), providers=["CPUExecutionProvider"]
    )
    with Image.open(request["image"]) as source:
        sheet = source.convert("RGB")
        images = [prepare(sheet, box) for box in request["boxes"]]

    # 不同文字行的长宽不同；同尺寸分批推理，结果按输入顺序返回
    groups = {}
    for index, pixels in enumerate(images):
        groups.setdefault(pixels.shape, []).append(index)
    results = [None] * len(images)
    for indexes in groups.values():
        for start in range(0, len(indexes), 8):
            part = indexes[start : start + 8]
            batch = np.stack([images[index] for index in part])
            prediction = session.run(None, {"x": batch})[0]
            for index, row in zip(part, decode(prediction, characters)):
                results[index] = row

    # 推理库可能向 stdout 写日志
    with output_path.open("w", encoding="utf-8") as output:
        json.dump(results, output, ensure_ascii=False)


if __name__ == "__main__":
    main()
