from dataclasses import dataclass


@dataclass(frozen=True)
class BuildVariant:
    all_sources: bool = True

    @property
    def name(self):
        return '聚合短剧'

    @property
    def slug(self):
        return 'juheduanju'

    @property
    def arguments(self):
        return []

    @property
    def flutter_arguments(self):
        return []

    @property
    def linker_flags(self):
        return '-s -w'

    @classmethod
    def from_dart_defines(cls, encoded):
        return cls()
